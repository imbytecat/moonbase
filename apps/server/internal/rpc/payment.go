package rpc

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"google.golang.org/protobuf/types/known/timestamppb"

	paymentv1 "github.com/imbytecat/moonbase/server/internal/gen/payment/v1"
	"github.com/imbytecat/moonbase/server/internal/gen/payment/v1/paymentv1connect"
	"github.com/imbytecat/moonbase/server/internal/pay"
	"github.com/imbytecat/moonbase/server/internal/repository"
)

const defaultPaymentPageSize = 20

// PaymentService owns the payment_orders state machine. Provider calls go
// through the pay.Gateway seam; every settlement write is status-guarded in
// SQL so replayed notifications and concurrent syncs stay idempotent.
type PaymentService struct {
	repo    repository.Querier
	gateway pay.Gateway
	logger  *slog.Logger
}

func NewPaymentService(repo repository.Querier, gateway pay.Gateway, logger *slog.Logger) *PaymentService {
	return &PaymentService{repo: repo, gateway: gateway, logger: logger}
}

var _ paymentv1connect.PaymentServiceHandler = (*PaymentService)(nil)

func (s *PaymentService) ListPaymentOptions(
	ctx context.Context,
	req *connect.Request[paymentv1.ListPaymentOptionsRequest],
) (*connect.Response[paymentv1.ListPaymentOptionsResponse], error) {
	if !pay.Purposes.Known(req.Msg.GetPurpose()) {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("unknown payment purpose %q", req.Msg.GetPurpose()))
	}
	options, err := s.gateway.OptionsFor(ctx, req.Msg.GetPurpose())
	if err != nil {
		return nil, s.internal(ctx, "list payment options", err)
	}
	out := make([]*paymentv1.PaymentOption, len(options))
	for i, o := range options {
		out[i] = &paymentv1.PaymentOption{ProfileId: o.ProfileID, Name: o.Name, Provider: o.Provider, Methods: o.Methods}
	}
	return connect.NewResponse(&paymentv1.ListPaymentOptionsResponse{Options: out}), nil
}

func (s *PaymentService) CreatePaymentOrder(
	ctx context.Context,
	req *connect.Request[paymentv1.CreatePaymentOrderRequest],
) (*connect.Response[paymentv1.CreatePaymentOrderResponse], error) {
	profile, err := s.gateway.ProfileFor(ctx, req.Msg.GetPurpose(), req.Msg.GetProfileId())
	if errors.Is(err, pay.ErrNotConfigured) {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			errors.New("payment profile is not bound to this purpose"))
	}
	if err != nil {
		return nil, s.internal(ctx, "resolve payment profile", err)
	}

	outTradeNo := newOutTradeNo()
	credential, err := s.gateway.Create(ctx, profile, pay.CreateRequest{
		OutTradeNo: outTradeNo,
		Subject:    req.Msg.GetSubject(),
		Amount:     req.Msg.GetAmount(),
		Method:     req.Msg.GetMethod(),
		PayerID:    req.Msg.GetPayerId(),
		ReturnURL:  req.Msg.GetReturnUrl(),
		ClientIP:   clientIP(req),
	})
	if err != nil {
		if errors.Is(err, pay.ErrMethodNotOffered) {
			return nil, connect.NewError(connect.CodeFailedPrecondition,
				errors.New("this payment method is not offered by the selected profile"))
		}
		if errors.Is(err, pay.ErrMissingInput) {
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		}
		return nil, connect.NewError(connect.CodeUnavailable, fmt.Errorf("create payment: %w", err))
	}

	row, err := s.repo.InsertPaymentOrder(ctx, repository.InsertPaymentOrderParams{
		OutTradeNo:  outTradeNo,
		Purpose:     req.Msg.GetPurpose(),
		ProfileID:   profile.Id,
		ProfileName: profile.Name,
		Provider:    profile.Provider,
		Method:      req.Msg.GetMethod(),
		Subject:     req.Msg.GetSubject(),
		Amount:      req.Msg.GetAmount(),
		Currency:    pay.Currency(profile.Provider),
		Credential:  credential,
	})
	if err != nil {
		return nil, s.internal(ctx, "insert payment order", err)
	}
	return connect.NewResponse(&paymentv1.CreatePaymentOrderResponse{Order: toProtoPaymentOrder(row)}), nil
}

func (s *PaymentService) GetPaymentOrder(
	ctx context.Context,
	req *connect.Request[paymentv1.GetPaymentOrderRequest],
) (*connect.Response[paymentv1.GetPaymentOrderResponse], error) {
	row, err := s.getOrder(ctx, req.Msg.GetId())
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&paymentv1.GetPaymentOrderResponse{Order: toProtoPaymentOrder(row)}), nil
}

func (s *PaymentService) SyncPaymentOrder(
	ctx context.Context,
	req *connect.Request[paymentv1.SyncPaymentOrderRequest],
) (*connect.Response[paymentv1.SyncPaymentOrderResponse], error) {
	row, err := s.getOrder(ctx, req.Msg.GetId())
	if err != nil {
		return nil, err
	}
	row, err = s.reconcile(ctx, row)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&paymentv1.SyncPaymentOrderResponse{Order: toProtoPaymentOrder(row)}), nil
}

// reconcile pulls the provider's current state and applies it through the
// status-guarded writes. Terminal states pass through untouched.
func (s *PaymentService) reconcile(ctx context.Context, row repository.PaymentOrder) (repository.PaymentOrder, error) {
	switch row.Status {
	case "created":
		profile, err := s.gateway.ProfileFor(ctx, row.Purpose, row.ProfileID)
		if err != nil {
			return row, s.syncUnavailable(err)
		}
		result, err := s.gateway.Query(ctx, profile, row.OutTradeNo)
		if err != nil {
			return row, s.syncUnavailable(err)
		}
		return s.applyQueryResult(ctx, row, result)
	case "refunding":
		profile, err := s.gateway.ProfileFor(ctx, row.Purpose, row.ProfileID)
		if err != nil {
			return row, s.syncUnavailable(err)
		}
		settled, err := s.gateway.QueryRefund(ctx, profile, refundNo(row.OutTradeNo))
		if err != nil {
			return row, s.syncUnavailable(err)
		}
		if !settled {
			return row, nil
		}
		updated, err := s.repo.MarkPaymentOrderRefunded(ctx, row.ID)
		return s.settled(ctx, row, updated, err)
	default:
		return row, nil
	}
}

func (s *PaymentService) applyQueryResult(ctx context.Context, row repository.PaymentOrder, result pay.QueryResult) (repository.PaymentOrder, error) {
	switch result.State {
	case pay.StatePaid:
		paidAt := result.PaidAt
		if paidAt.IsZero() {
			paidAt = time.Now()
		}
		updated, err := s.repo.MarkPaymentOrderPaid(ctx, repository.MarkPaymentOrderPaidParams{
			ID:              row.ID,
			ProviderTradeNo: result.ProviderTradeNo,
			PayerID:         result.PayerID,
			PaidAt:          pgtype.Timestamptz{Time: paidAt, Valid: true},
		})
		return s.settled(ctx, row, updated, err)
	case pay.StateClosed:
		updated, err := s.repo.MarkPaymentOrderClosed(ctx, row.ID)
		return s.settled(ctx, row, updated, err)
	case pay.StateRefunded:
		updated, err := s.repo.MarkPaymentOrderRefunded(ctx, row.ID)
		return s.settled(ctx, row, updated, err)
	default:
		return row, nil
	}
}

// settled folds the status-guarded UPDATE result: zero rows (ErrNoRows)
// means another writer settled first — reread and return the current truth.
func (s *PaymentService) settled(ctx context.Context, row, updated repository.PaymentOrder, err error) (repository.PaymentOrder, error) {
	if err == nil {
		return updated, nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		current, rerr := s.repo.GetPaymentOrder(ctx, row.ID)
		if rerr != nil {
			return row, s.internal(ctx, "reread payment order", rerr)
		}
		return current, nil
	}
	return row, s.internal(ctx, "update payment order", err)
}

func (s *PaymentService) syncUnavailable(err error) error {
	if errors.Is(err, pay.ErrNotConfigured) {
		return connect.NewError(connect.CodeFailedPrecondition,
			errors.New("payment profile is no longer bound to this purpose"))
	}
	return connect.NewError(connect.CodeUnavailable, fmt.Errorf("query payment provider: %w", err))
}

func (s *PaymentService) ListPaymentOrders(
	ctx context.Context,
	req *connect.Request[paymentv1.ListPaymentOrdersRequest],
) (*connect.Response[paymentv1.ListPaymentOrdersResponse], error) {
	pageSize := req.Msg.GetPageSize()
	if pageSize == 0 {
		pageSize = defaultPaymentPageSize
	}
	var status, provider pgtype.Text
	if v := req.Msg.GetStatus(); v != "" {
		status = pgtype.Text{String: v, Valid: true}
	}
	if v := req.Msg.GetProvider(); v != "" {
		provider = pgtype.Text{String: v, Valid: true}
	}
	rows, err := s.repo.ListPaymentOrders(ctx, repository.ListPaymentOrdersParams{
		Limit:    pageSize,
		Offset:   req.Msg.GetPage() * pageSize,
		Status:   status,
		Provider: provider,
	})
	if err != nil {
		return nil, s.internal(ctx, "list payment orders", err)
	}
	total, err := s.repo.CountPaymentOrders(ctx, repository.CountPaymentOrdersParams{
		Status:   status,
		Provider: provider,
	})
	if err != nil {
		return nil, s.internal(ctx, "count payment orders", err)
	}
	orders := make([]*paymentv1.PaymentOrder, len(rows))
	for i, row := range rows {
		orders[i] = toProtoPaymentOrder(row)
	}
	return connect.NewResponse(&paymentv1.ListPaymentOrdersResponse{Orders: orders, Total: total}), nil
}

func (s *PaymentService) RefundPaymentOrder(
	ctx context.Context,
	req *connect.Request[paymentv1.RefundPaymentOrderRequest],
) (*connect.Response[paymentv1.RefundPaymentOrderResponse], error) {
	row, err := s.getOrder(ctx, req.Msg.GetId())
	if err != nil {
		return nil, err
	}
	if row.Status != "paid" {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("order is %s, only paid orders can be refunded", row.Status))
	}
	profile, err := s.gateway.ProfileFor(ctx, row.Purpose, row.ProfileID)
	if err != nil {
		return nil, s.syncUnavailable(err)
	}
	result, err := s.gateway.Refund(ctx, profile, pay.RefundRequest{
		OutTradeNo: row.OutTradeNo,
		RefundNo:   refundNo(row.OutTradeNo),
		Reason:     req.Msg.GetReason(),
		Amount:     row.Amount,
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeUnavailable, fmt.Errorf("refund: %w", err))
	}
	var updated repository.PaymentOrder
	if result.Settled {
		updated, err = s.repo.MarkPaymentOrderRefunded(ctx, row.ID)
	} else {
		updated, err = s.repo.MarkPaymentOrderRefunding(ctx, row.ID)
	}
	updated, err = s.settled(ctx, row, updated, err)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&paymentv1.RefundPaymentOrderResponse{Order: toProtoPaymentOrder(updated)}), nil
}

func (s *PaymentService) getOrder(ctx context.Context, id string) (repository.PaymentOrder, error) {
	orderID, err := uuid.Parse(id)
	if err != nil {
		return repository.PaymentOrder{}, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid order id"))
	}
	row, err := s.repo.GetPaymentOrder(ctx, orderID)
	if errors.Is(err, pgx.ErrNoRows) {
		return repository.PaymentOrder{}, connect.NewError(connect.CodeNotFound, errors.New("payment order not found"))
	}
	if err != nil {
		return repository.PaymentOrder{}, s.internal(ctx, "get payment order", err)
	}
	return row, nil
}

// refundNo derives the merchant refund number from the order number — full
// refunds only, so one deterministic id per order keeps retries idempotent
// (providers dedupe by out_refund_no).
func refundNo(outTradeNo string) string {
	return outTradeNo + "R"
}

func newOutTradeNo() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return fmt.Sprintf("PO%s%x", time.Now().Format("20060102150405"), b)
}

// clientIP prefers the reverse-proxy header, falling back to the RPC peer —
// WeChat h5 prepay requires the PAYER's IP, which in the demo is the admin
// driving the checkout.
func clientIP[T any](req *connect.Request[T]) string {
	if fwd := req.Header().Get("X-Forwarded-For"); fwd != "" {
		if ip, _, ok := strings.Cut(fwd, ","); ok {
			return strings.TrimSpace(ip)
		}
		return strings.TrimSpace(fwd)
	}
	if host, _, err := net.SplitHostPort(req.Peer().Addr); err == nil {
		return host
	}
	return req.Peer().Addr
}

func toProtoPaymentOrder(row repository.PaymentOrder) *paymentv1.PaymentOrder {
	out := &paymentv1.PaymentOrder{
		Id:              row.ID.String(),
		OutTradeNo:      row.OutTradeNo,
		Purpose:         row.Purpose,
		ProfileId:       row.ProfileID,
		ProfileName:     row.ProfileName,
		Provider:        row.Provider,
		Method:          row.Method,
		Subject:         row.Subject,
		Amount:          row.Amount,
		Currency:        row.Currency,
		Status:          row.Status,
		ProviderTradeNo: row.ProviderTradeNo,
		PayerId:         row.PayerID,
		Credential:      row.Credential,
		CredentialKind:  string(pay.KindOf(row.Provider, row.Method)),
		CreatedAt:       timestamppb.New(row.CreatedAt),
		UpdatedAt:       timestamppb.New(row.UpdatedAt),
	}
	if row.PaidAt.Valid {
		out.PaidAt = timestamppb.New(row.PaidAt.Time)
	}
	return out
}

func (s *PaymentService) internal(ctx context.Context, op string, err error) error {
	s.logger.ErrorContext(ctx, "rpc failed", "op", op, "error", err)
	return connect.NewError(connect.CodeInternal, errors.New("internal error"))
}
