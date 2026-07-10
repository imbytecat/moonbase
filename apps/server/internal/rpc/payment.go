package rpc

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"slices"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"google.golang.org/protobuf/types/known/timestamppb"

	kitsettings "github.com/imbytecat/moonbase/integrations/core/settings"
	paymentv1 "github.com/imbytecat/moonbase/server/internal/gen/payment/v1"
	"github.com/imbytecat/moonbase/server/internal/gen/payment/v1/paymentv1connect"
	systemv1 "github.com/imbytecat/moonbase/server/internal/gen/system/v1"
	"github.com/imbytecat/moonbase/server/internal/pay"
	"github.com/imbytecat/moonbase/server/internal/repository"
)

const defaultPaymentPageSize = 20

type paymentCore struct {
	repo     repository.Querier
	gateway  pay.Gateway
	checkout *pay.CheckoutManager
	logger   *slog.Logger
}

type PaymentService struct {
	core *paymentCore
}

type PaymentCheckoutService struct {
	core *paymentCore
}

func NewPaymentService(repo repository.Querier, gateway pay.Gateway, checkout *pay.CheckoutManager, logger *slog.Logger) *PaymentService {
	return &PaymentService{core: &paymentCore{repo: repo, gateway: gateway, checkout: checkout, logger: logger}}
}

func NewPaymentCheckoutService(repo repository.Querier, gateway pay.Gateway, checkout *pay.CheckoutManager, logger *slog.Logger) *PaymentCheckoutService {
	return &PaymentCheckoutService{core: &paymentCore{repo: repo, gateway: gateway, checkout: checkout, logger: logger}}
}

var _ paymentv1connect.PaymentServiceHandler = (*PaymentService)(nil)
var _ paymentv1connect.PaymentCheckoutServiceHandler = (*PaymentCheckoutService)(nil)

func (s *PaymentService) CreateDemoCheckout(
	ctx context.Context,
	req *connect.Request[paymentv1.CreateDemoCheckoutRequest],
) (*connect.Response[paymentv1.CreateDemoCheckoutResponse], error) {
	issued, err := s.core.checkout.Create(ctx, pay.CheckoutCommand{
		Purpose: pay.PurposeCheckout, BusinessReference: req.Msg.GetBusinessReference(),
		IdempotencyKey: req.Msg.GetIdempotencyKey(), Subject: req.Msg.GetSubject(),
		Amount: req.Msg.GetAmount(), ReturnPath: req.Msg.GetReturnPath(),
	})
	if errors.Is(err, pay.ErrIdempotencyConflict) {
		return nil, connect.NewError(connect.CodeAlreadyExists, err)
	}
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	return connect.NewResponse(&paymentv1.CreateDemoCheckoutResponse{CheckoutUrl: issued.CheckoutURL}), nil
}

func (s *PaymentCheckoutService) GetCheckoutSession(
	ctx context.Context,
	req *connect.Request[paymentv1.GetCheckoutSessionRequest],
) (*connect.Response[paymentv1.GetCheckoutSessionResponse], error) {
	session, err := s.core.resolveCheckout(ctx, req.Msg.GetSession())
	if err != nil {
		return nil, err
	}
	methods, err := s.core.availableMethods(ctx, session.Purpose)
	if err != nil {
		return nil, s.core.internal(ctx, "list checkout methods", err)
	}
	out := &paymentv1.CheckoutSession{
		CheckoutUrl: s.core.checkout.CheckoutURL(req.Msg.GetSession()),
		Subject:     session.Subject, Amount: session.Amount, Status: session.Status,
		ReturnPath: session.ReturnPath, ExpiresAt: timestamppb.New(session.ExpiresAt),
		PaymentMethods: methods, PaymentMethod: session.PaymentMethod,
	}
	if order, err := s.core.repo.GetPaymentOrderByCheckoutSession(ctx, pgtype.Text{String: session.ID, Valid: true}); err == nil {
		out.Status = order.Status
		out.Order = checkoutOrderToProto(order, req.Msg.GetSession())
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return nil, s.core.internal(ctx, "get checkout order", err)
	}
	return connect.NewResponse(&paymentv1.GetCheckoutSessionResponse{Session: out}), nil
}

func (s *PaymentCheckoutService) PlanCheckout(
	ctx context.Context,
	req *connect.Request[paymentv1.PlanCheckoutRequest],
) (*connect.Response[paymentv1.PlanCheckoutResponse], error) {
	session, err := s.core.resolveCheckout(ctx, req.Msg.GetSession())
	if err != nil {
		return nil, err
	}
	if session.Status != "open" {
		if session.PaymentMethod != req.Msg.GetPaymentMethod() {
			return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("checkout path is already locked"))
		}
		descriptor, ok := pay.Describe(session.Provider)
		if !ok {
			return nil, connect.NewError(connect.CodeFailedPrecondition, pay.ErrNotConfigured)
		}
		index := slices.IndexFunc(descriptor.Products, func(product pay.ProductDescriptor) bool {
			return product.ID == session.ProductID
		})
		if index < 0 {
			return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("planned payment product is unavailable"))
		}
		return connect.NewResponse(planCheckoutResponse(req.Msg.GetPaymentMethod(), descriptor.Products[index])), nil
	}
	profile, plan, err := s.core.route(ctx, session.Purpose, req.Msg.GetPaymentMethod(), clientContext(req))
	if err != nil {
		return nil, err
	}
	_, err = s.core.repo.PlanCheckoutSession(ctx, repository.PlanCheckoutSessionParams{
		ID: session.ID, PaymentMethod: req.Msg.GetPaymentMethod(), ProfileID: profile.Id,
		ProfileName: profile.Name, Provider: profile.Provider, ProductID: plan.ProductID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("checkout path is already locked"))
	}
	if err != nil {
		return nil, s.core.internal(ctx, "plan checkout", err)
	}
	return connect.NewResponse(planCheckoutResponse(req.Msg.GetPaymentMethod(), pay.ProductDescriptor{Input: plan.Input})), nil
}

func planCheckoutResponse(method string, product pay.ProductDescriptor) *paymentv1.PlanCheckoutResponse {
	js, ui := product.Input.JSONForm()
	return &paymentv1.PlanCheckoutResponse{
		PaymentMethod: method,
		Input:         &systemv1.ProviderForm{Schema: toStruct(js), UiSchema: toStruct(ui)},
	}
}

func (s *PaymentCheckoutService) ConfirmCheckout(
	ctx context.Context,
	req *connect.Request[paymentv1.ConfirmCheckoutRequest],
) (*connect.Response[paymentv1.ConfirmCheckoutResponse], error) {
	session, err := s.core.resolveCheckout(ctx, req.Msg.GetSession())
	if err != nil {
		return nil, err
	}
	if (session.Status != "planned" && session.Status != "confirmed") || session.PaymentMethod != req.Msg.GetPaymentMethod() {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("checkout must be planned before confirmation"))
	}
	inputs := map[string]any{}
	if req.Msg.GetInputs() != nil {
		inputs = req.Msg.GetInputs().AsMap()
	}
	descriptor, ok := pay.Describe(session.Provider)
	if !ok {
		return nil, connect.NewError(connect.CodeFailedPrecondition, pay.ErrNotConfigured)
	}
	product := slices.IndexFunc(descriptor.Products, func(product pay.ProductDescriptor) bool {
		return product.ID == session.ProductID
	})
	if product < 0 {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("planned payment product is unavailable"))
	}
	if err := descriptor.Products[product].Input.Validate(inputs); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	inputJSON, err := json.Marshal(inputs)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	started, err := s.core.repo.StartCheckoutOrder(ctx, repository.StartCheckoutOrderParams{
		ID: session.ID, OutTradeNo: newOutTradeNo(), Inputs: inputJSON,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("checkout session cannot create an order"))
	}
	if err != nil {
		return nil, s.core.internal(ctx, "start checkout order", err)
	}
	order := paymentOrderFromStart(started)
	if order.Status == "creating" {
		order, err = s.core.createProviderOrder(ctx, order, inputs, session.ReturnPath, clientContext(req))
		if err != nil {
			return nil, err
		}
	}
	return connect.NewResponse(&paymentv1.ConfirmCheckoutResponse{
		Order: checkoutOrderToProto(order, req.Msg.GetSession()),
	}), nil
}

func (s *PaymentCheckoutService) GetCheckoutOrder(
	ctx context.Context,
	req *connect.Request[paymentv1.GetCheckoutOrderRequest],
) (*connect.Response[paymentv1.GetCheckoutOrderResponse], error) {
	session, err := s.core.resolveCheckout(ctx, req.Msg.GetSession())
	if err != nil {
		return nil, err
	}
	order, err := s.core.repo.GetPaymentOrderByCheckoutSession(ctx, pgtype.Text{String: session.ID, Valid: true})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("checkout order not found"))
	}
	if err != nil {
		return nil, s.core.internal(ctx, "get checkout order", err)
	}
	order, err = s.core.reconcile(ctx, order, session.ReturnPath, clientContext(req))
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&paymentv1.GetCheckoutOrderResponse{
		Order: checkoutOrderToProto(order, req.Msg.GetSession()),
	}), nil
}

func (c *paymentCore) route(ctx context.Context, purpose, method string, client pay.ClientContext) (kitsettings.GenericProfile, pay.PlanResult, error) {
	profiles, err := c.gateway.ProfilesFor(ctx, purpose)
	if err != nil {
		return kitsettings.GenericProfile{}, pay.PlanResult{}, err
	}
	for _, profile := range profiles {
		plan, err := c.gateway.Plan(ctx, profile, pay.PlanRequest{PaymentMethod: method, Client: client})
		if err == nil {
			return profile, plan, nil
		}
		if !errors.Is(err, pay.ErrUnknownMethod) && !errors.Is(err, pay.ErrMethodNotOffered) {
			return kitsettings.GenericProfile{}, pay.PlanResult{}, c.unavailable(err)
		}
	}
	return kitsettings.GenericProfile{}, pay.PlanResult{}, connect.NewError(connect.CodeFailedPrecondition,
		fmt.Errorf("payment method %q is not available", method))
}

func (c *paymentCore) availableMethods(ctx context.Context, purpose string) ([]*paymentv1.CheckoutPaymentMethod, error) {
	profiles, err := c.gateway.ProfilesFor(ctx, purpose)
	if err != nil {
		return nil, err
	}
	seen := map[string]struct{}{}
	var out []*paymentv1.CheckoutPaymentMethod
	for _, profile := range profiles {
		descriptor, ok := pay.Describe(profile.Provider)
		if !ok {
			continue
		}
		offered := pay.ProfileProducts(profile)
		for _, method := range descriptor.Methods {
			if _, ok := seen[method.Key]; ok {
				continue
			}
			if !slices.ContainsFunc(descriptor.Products, func(product pay.ProductDescriptor) bool {
				return product.Method == method.Key && slices.Contains(offered, product.ID)
			}) {
				continue
			}
			seen[method.Key] = struct{}{}
			out = append(out, &paymentv1.CheckoutPaymentMethod{
				Key: method.Key, Presentation: presentationToProto(method.Presentation),
			})
		}
	}
	return out, nil
}

func (c *paymentCore) createProviderOrder(ctx context.Context, order repository.PaymentOrder, inputs map[string]any, returnPath string, client pay.ClientContext) (repository.PaymentOrder, error) {
	profile, err := c.gateway.ProfileByID(ctx, order.ProfileID)
	if err != nil {
		return order, c.unavailable(err)
	}
	action, err := c.gateway.Create(ctx, profile, pay.CreateRequest{
		OutTradeNo: order.OutTradeNo, Subject: order.Subject, Amount: order.Amount,
		ProductID: order.ProductID, Inputs: inputs,
		ReturnURL: c.checkout.ReturnURL(returnPath), Client: client,
	})
	if err != nil {
		return order, c.unavailable(fmt.Errorf("create payment: %w", err))
	}
	raw, err := json.Marshal(action)
	if err != nil {
		return order, c.internal(ctx, "encode payment action", err)
	}
	expiresAt := pgtype.Timestamptz{}
	if action.QR != nil && !action.QR.ExpiresAt.IsZero() {
		expiresAt = pgtype.Timestamptz{Time: action.QR.ExpiresAt, Valid: true}
	}
	updated, err := c.repo.SetPaymentOrderPending(ctx, repository.SetPaymentOrderPendingParams{
		ID: order.ID, Action: raw, ActionExpiresAt: expiresAt,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return c.repo.GetPaymentOrder(ctx, order.ID)
	}
	if err != nil {
		return order, c.internal(ctx, "store payment action", err)
	}
	return updated, nil
}

func (c *paymentCore) reconcile(ctx context.Context, order repository.PaymentOrder, returnPath string, client pay.ClientContext) (repository.PaymentOrder, error) {
	switch order.Status {
	case "creating":
		profile, err := c.gateway.ProfileByID(ctx, order.ProfileID)
		if err != nil {
			return order, c.unavailable(err)
		}
		result, err := c.gateway.Query(ctx, profile, order.OutTradeNo)
		if err != nil {
			return order, c.unavailable(err)
		}
		if result.Exists && result.State != pay.StatePending {
			return c.applyQueryResult(ctx, order, result)
		}
		inputs := map[string]any{}
		if len(order.Inputs) > 0 {
			_ = json.Unmarshal(order.Inputs, &inputs)
		}
		return c.createProviderOrder(ctx, order, inputs, returnPath, client)
	case "pending":
		profile, err := c.gateway.ProfileByID(ctx, order.ProfileID)
		if err != nil {
			return order, c.unavailable(err)
		}
		result, err := c.gateway.Query(ctx, profile, order.OutTradeNo)
		if err != nil {
			return order, c.unavailable(err)
		}
		return c.applyQueryResult(ctx, order, result)
	case "refunding":
		profile, err := c.gateway.ProfileByID(ctx, order.ProfileID)
		if err != nil {
			return order, c.unavailable(err)
		}
		settled, err := c.gateway.QueryRefund(ctx, profile, refundNo(order.OutTradeNo))
		if err != nil {
			return order, c.unavailable(err)
		}
		if settled {
			return c.markRefunded(ctx, order)
		}
	}
	return order, nil
}

func (c *paymentCore) applyQueryResult(ctx context.Context, order repository.PaymentOrder, result pay.QueryResult) (repository.PaymentOrder, error) {
	switch result.State {
	case pay.StatePaid:
		paidAt := result.PaidAt
		if paidAt.IsZero() {
			paidAt = time.Now()
		}
		_, err := c.repo.MarkPaymentOrderPaid(ctx, repository.MarkPaymentOrderPaidParams{
			ID: order.ID, ProviderTradeNo: result.ProviderTradeNo, PayerID: result.PayerID,
			PaidAt: pgtype.Timestamptz{Time: paidAt, Valid: true},
		})
		return c.rereadTransition(ctx, order, err)
	case pay.StateClosed:
		updated, err := c.repo.MarkPaymentOrderClosed(ctx, order.ID)
		return c.settled(ctx, order, updated, err)
	case pay.StateRefunded:
		return c.markRefunded(ctx, order)
	default:
		return order, nil
	}
}

func (c *paymentCore) markRefunded(ctx context.Context, order repository.PaymentOrder) (repository.PaymentOrder, error) {
	_, err := c.repo.MarkPaymentOrderRefunded(ctx, order.ID)
	return c.rereadTransition(ctx, order, err)
}

func (c *paymentCore) rereadTransition(ctx context.Context, order repository.PaymentOrder, err error) (repository.PaymentOrder, error) {
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return order, c.internal(ctx, "update payment order", err)
	}
	current, readErr := c.repo.GetPaymentOrder(ctx, order.ID)
	if readErr != nil {
		return order, c.internal(ctx, "reread payment order", readErr)
	}
	return current, nil
}

func (c *paymentCore) settled(ctx context.Context, order, updated repository.PaymentOrder, err error) (repository.PaymentOrder, error) {
	if err == nil {
		return updated, nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return c.rereadTransition(ctx, order, nil)
	}
	return order, c.internal(ctx, "update payment order", err)
}

func (s *PaymentService) GetPaymentOrder(ctx context.Context, req *connect.Request[paymentv1.GetPaymentOrderRequest]) (*connect.Response[paymentv1.GetPaymentOrderResponse], error) {
	order, err := s.core.getOrder(ctx, req.Msg.GetId())
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&paymentv1.GetPaymentOrderResponse{Order: paymentOrderToProto(order)}), nil
}

func (s *PaymentService) SyncPaymentOrder(ctx context.Context, req *connect.Request[paymentv1.SyncPaymentOrderRequest]) (*connect.Response[paymentv1.SyncPaymentOrderResponse], error) {
	order, err := s.core.getOrder(ctx, req.Msg.GetId())
	if err != nil {
		return nil, err
	}
	order, err = s.core.reconcile(ctx, order, "", clientContext(req))
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&paymentv1.SyncPaymentOrderResponse{Order: paymentOrderToProto(order)}), nil
}

func (s *PaymentService) ListPaymentOrders(ctx context.Context, req *connect.Request[paymentv1.ListPaymentOrdersRequest]) (*connect.Response[paymentv1.ListPaymentOrdersResponse], error) {
	pageSize := req.Msg.GetPageSize()
	if pageSize == 0 {
		pageSize = defaultPaymentPageSize
	}
	var status, provider pgtype.Text
	if req.Msg.GetStatus() != "" {
		status = pgtype.Text{String: req.Msg.GetStatus(), Valid: true}
	}
	if req.Msg.GetProvider() != "" {
		provider = pgtype.Text{String: req.Msg.GetProvider(), Valid: true}
	}
	rows, err := s.core.repo.ListPaymentOrders(ctx, repository.ListPaymentOrdersParams{
		Limit: pageSize, Offset: req.Msg.GetPage() * pageSize, Status: status, Provider: provider,
	})
	if err != nil {
		return nil, s.core.internal(ctx, "list payment orders", err)
	}
	total, err := s.core.repo.CountPaymentOrders(ctx, repository.CountPaymentOrdersParams{Status: status, Provider: provider})
	if err != nil {
		return nil, s.core.internal(ctx, "count payment orders", err)
	}
	orders := make([]*paymentv1.PaymentOrder, len(rows))
	for i, row := range rows {
		orders[i] = paymentOrderToProto(row)
	}
	return connect.NewResponse(&paymentv1.ListPaymentOrdersResponse{Orders: orders, Total: total}), nil
}

func (s *PaymentService) RefundPaymentOrder(ctx context.Context, req *connect.Request[paymentv1.RefundPaymentOrderRequest]) (*connect.Response[paymentv1.RefundPaymentOrderResponse], error) {
	order, err := s.core.getOrder(ctx, req.Msg.GetId())
	if err != nil {
		return nil, err
	}
	if order.Status != "paid" {
		return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("order is %s, only paid orders can be refunded", order.Status))
	}
	profile, err := s.core.gateway.ProfileByID(ctx, order.ProfileID)
	if err != nil {
		return nil, s.core.unavailable(err)
	}
	result, err := s.core.gateway.Refund(ctx, profile, pay.RefundRequest{
		OutTradeNo: order.OutTradeNo, RefundNo: refundNo(order.OutTradeNo), Reason: req.Msg.GetReason(), Amount: order.Amount,
	})
	if err != nil {
		return nil, s.core.unavailable(fmt.Errorf("refund: %w", err))
	}
	if result.Settled {
		order, err = s.core.markRefunded(ctx, order)
	} else {
		updated, updateErr := s.core.repo.MarkPaymentOrderRefunding(ctx, order.ID)
		order, err = s.core.settled(ctx, order, updated, updateErr)
	}
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&paymentv1.RefundPaymentOrderResponse{Order: paymentOrderToProto(order)}), nil
}

func (c *paymentCore) resolveCheckout(ctx context.Context, token string) (repository.PaymentCheckoutSession, error) {
	session, err := c.checkout.Resolve(ctx, token)
	switch {
	case errors.Is(err, pay.ErrInvalidCheckout):
		return session, connect.NewError(connect.CodeNotFound, err)
	case errors.Is(err, pay.ErrCheckoutExpired):
		return session, connect.NewError(connect.CodeFailedPrecondition, err)
	case err != nil:
		return session, c.internal(ctx, "resolve checkout", err)
	default:
		return session, nil
	}
}

func (c *paymentCore) getOrder(ctx context.Context, id string) (repository.PaymentOrder, error) {
	orderID, err := uuid.Parse(id)
	if err != nil {
		return repository.PaymentOrder{}, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid order id"))
	}
	order, err := c.repo.GetPaymentOrder(ctx, orderID)
	if errors.Is(err, pgx.ErrNoRows) {
		return order, connect.NewError(connect.CodeNotFound, errors.New("payment order not found"))
	}
	if err != nil {
		return order, c.internal(ctx, "get payment order", err)
	}
	return order, nil
}

func (c *paymentCore) unavailable(err error) error {
	if errors.Is(err, pay.ErrNotConfigured) {
		return connect.NewError(connect.CodeFailedPrecondition, err)
	}
	return connect.NewError(connect.CodeUnavailable, err)
}

func (c *paymentCore) internal(ctx context.Context, op string, err error) error {
	c.logger.ErrorContext(ctx, "rpc failed", "op", op, "error", err)
	return connect.NewError(connect.CodeInternal, errors.New("internal error"))
}

func refundNo(outTradeNo string) string { return outTradeNo + "R" }

func newOutTradeNo() string {
	var value [8]byte
	_, _ = rand.Read(value[:])
	return fmt.Sprintf("PO%s%x", time.Now().Format("20060102150405"), value)
}

func clientContext[T any](req *connect.Request[T]) pay.ClientContext {
	return pay.ClientContext{UserAgent: req.Header().Get("User-Agent"), IP: clientIP(req)}
}

func clientIP[T any](req *connect.Request[T]) string {
	if forwarded := req.Header().Get("X-Forwarded-For"); forwarded != "" {
		ip, _, _ := strings.Cut(forwarded, ",")
		return strings.TrimSpace(ip)
	}
	if host, _, err := net.SplitHostPort(req.Peer().Addr); err == nil {
		return host
	}
	return req.Peer().Addr
}

func paymentOrderFromStart(row repository.StartCheckoutOrderRow) repository.PaymentOrder {
	return repository.PaymentOrder(row)
}

func paymentOrderToProto(row repository.PaymentOrder) *paymentv1.PaymentOrder {
	out := &paymentv1.PaymentOrder{
		Id: row.ID.String(), OutTradeNo: row.OutTradeNo, Purpose: row.Purpose,
		BusinessReference: row.BusinessReference, ProfileId: row.ProfileID,
		ProfileName: row.ProfileName, Provider: row.Provider, PaymentMethod: row.PaymentMethod,
		ProductId: row.ProductID, Subject: row.Subject, Amount: row.Amount, Currency: row.Currency,
		Status: row.Status, ProviderTradeNo: row.ProviderTradeNo, PayerId: row.PayerID,
		FailureReason: row.FailureReason, CreatedAt: timestamppb.New(row.CreatedAt),
		UpdatedAt: timestamppb.New(row.UpdatedAt),
	}
	if row.PaidAt.Valid {
		out.PaidAt = timestamppb.New(row.PaidAt.Time)
	}
	return out
}

func checkoutOrderToProto(row repository.PaymentOrder, token string) *paymentv1.CheckoutOrder {
	return &paymentv1.CheckoutOrder{Order: paymentOrderToProto(row), Action: actionToProto(row, token)}
}

func actionToProto(row repository.PaymentOrder, token string) *paymentv1.PaymentAction {
	if len(row.Action) == 0 {
		return nil
	}
	var action pay.Action
	if json.Unmarshal(row.Action, &action) != nil {
		return nil
	}
	out := &paymentv1.PaymentAction{}
	switch {
	case action.QR != nil:
		qr := &paymentv1.QrAction{Data: action.QR.Data}
		if !action.QR.ExpiresAt.IsZero() {
			qr.ExpiresAt = timestamppb.New(action.QR.ExpiresAt)
		}
		out.Action = &paymentv1.PaymentAction_Qr{Qr: qr}
	case action.Redirect != nil:
		out.Action = &paymentv1.PaymentAction_Redirect{Redirect: &paymentv1.RedirectAction{Url: action.Redirect.URL}}
	case action.Form != nil:
		out.Action = &paymentv1.PaymentAction_Form{Form: &paymentv1.FormAction{Url: action.Form.URL, Method: action.Form.Method, Fields: action.Form.Fields}}
	case action.Wait != nil:
		out.Action = &paymentv1.PaymentAction_Wait{Wait: &paymentv1.WaitAction{PollAfterMs: action.Wait.PollAfterMS}}
	case action.HostedFlow != nil:
		out.Action = &paymentv1.PaymentAction_HostedFlow{HostedFlow: &paymentv1.HostedFlowAction{Url: "/api/payment/hosted-flow/" + token}}
	default:
		return nil
	}
	return out
}
