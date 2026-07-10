package pay

import (
	"context"
	"net/http"
	"strings"

	"github.com/imbytecat/moonbase/integrations/core/integration"
	kitsettings "github.com/imbytecat/moonbase/integrations/core/settings"
	payment "github.com/imbytecat/moonbase/integrations/payment"
	"github.com/imbytecat/moonbase/server/internal/settings"
)

const PurposeCheckout = "checkout"

var Purposes = integration.Catalog{{
	Key: PurposeCheckout, Name: "在线收款", Description: "业务订单使用的托管收银台", Cardinality: integration.Multiple,
}}

var Registry = payment.Registry

var (
	ErrNotConfigured    = payment.ErrNotConfigured
	ErrUnknownMethod    = payment.ErrUnknownMethod
	ErrMethodNotOffered = payment.ErrMethodNotOffered
	ErrMissingInput     = payment.ErrMissingInput
)

type State = payment.State

const (
	StatePending  = payment.StatePending
	StatePaid     = payment.StatePaid
	StateClosed   = payment.StateClosed
	StateRefunded = payment.StateRefunded
)

type ClientContext = payment.ClientContext
type PlanRequest = payment.PlanRequest
type PlanResult = payment.PlanResult
type ProviderDescriptor = payment.ProviderDescriptor
type ProductDescriptor = payment.ProductDescriptor
type CreateRequest = payment.CreateRequest
type Action = payment.Action
type QRAction = payment.QRAction
type RedirectAction = payment.RedirectAction
type FormAction = payment.FormAction
type WaitAction = payment.WaitAction
type HostedFlowAction = payment.HostedFlowAction
type QueryResult = payment.QueryResult
type RefundRequest = payment.RefundRequest
type RefundResult = payment.RefundResult
type NotifyResult = payment.NotifyResult

type Gateway interface {
	ProfilesFor(ctx context.Context, purpose string) ([]kitsettings.GenericProfile, error)
	ProfileByID(ctx context.Context, profileID string) (kitsettings.GenericProfile, error)
	Plan(ctx context.Context, profile kitsettings.GenericProfile, req PlanRequest) (PlanResult, error)
	Create(ctx context.Context, profile kitsettings.GenericProfile, req CreateRequest) (Action, error)
	Query(ctx context.Context, profile kitsettings.GenericProfile, outTradeNo string) (QueryResult, error)
	Refund(ctx context.Context, profile kitsettings.GenericProfile, req RefundRequest) (RefundResult, error)
	QueryRefund(ctx context.Context, profile kitsettings.GenericProfile, refundNo string) (settled bool, err error)
	ParseNotify(ctx context.Context, profile kitsettings.GenericProfile, r *http.Request) (NotifyResult, error)
}

func Providers() []string { return payment.Providers() }
func ProfileUsable(profile kitsettings.GenericProfile) bool {
	return payment.ProfileUsable(profile)
}
func Describe(provider string) (ProviderDescriptor, bool) { return payment.Describe(provider) }
func ProfileProducts(profile kitsettings.GenericProfile) []string {
	return payment.ProfileProducts(profile)
}
func ProfileConfiguredProducts(profile kitsettings.GenericProfile) []string {
	return payment.ProfileConfiguredProducts(profile)
}
func ValidateProducts(provider string, products []string) error {
	return payment.ValidateProducts(provider, products)
}
func Currency(provider string) string { return payment.Currency(provider) }
func RenderHostedFlow(provider, product, payload string) ([]byte, error) {
	return payment.RenderHostedFlow(provider, product, payload)
}

type Client struct {
	store     *settings.Store
	publicURL string
}

func NewClient(store *settings.Store, publicURL string) *Client {
	return &Client{store: store, publicURL: strings.TrimSuffix(publicURL, "/")}
}

var _ Gateway = (*Client)(nil)

func (c *Client) notifyURL(profile kitsettings.GenericProfile) string {
	return c.publicURL + "/api/payment/notify/" + profile.Provider + "/" + profile.Id
}

func (c *Client) ProfilesFor(ctx context.Context, purpose string) ([]kitsettings.GenericProfile, error) {
	cfg, err := c.store.Payment(ctx)
	if err != nil {
		return nil, err
	}
	bound := cfg.ProfilesFor(purpose)
	out := make([]kitsettings.GenericProfile, 0, len(bound))
	for _, profile := range bound {
		if ProfileUsable(profile) {
			out = append(out, profile)
		}
	}
	return out, nil
}

func (c *Client) ProfileByID(ctx context.Context, profileID string) (kitsettings.GenericProfile, error) {
	cfg, err := c.store.Payment(ctx)
	if err != nil {
		return kitsettings.GenericProfile{}, err
	}
	if profile, ok := cfg.Profile(profileID); ok && ProfileUsable(profile) {
		return profile, nil
	}
	return kitsettings.GenericProfile{}, ErrNotConfigured
}

func (c *Client) Plan(ctx context.Context, profile kitsettings.GenericProfile, req PlanRequest) (PlanResult, error) {
	return payment.Plan(ctx, profile, req)
}

func (c *Client) Create(ctx context.Context, profile kitsettings.GenericProfile, req CreateRequest) (Action, error) {
	return payment.Create(ctx, profile, req, c.notifyURL(profile))
}

func (c *Client) Query(ctx context.Context, profile kitsettings.GenericProfile, outTradeNo string) (QueryResult, error) {
	return payment.Query(ctx, profile, outTradeNo)
}

func (c *Client) Refund(ctx context.Context, profile kitsettings.GenericProfile, req RefundRequest) (RefundResult, error) {
	return payment.Refund(ctx, profile, req, c.notifyURL(profile))
}

func (c *Client) QueryRefund(ctx context.Context, profile kitsettings.GenericProfile, refundNo string) (bool, error) {
	return payment.QueryRefund(ctx, profile, refundNo)
}

func (c *Client) ParseNotify(ctx context.Context, profile kitsettings.GenericProfile, r *http.Request) (NotifyResult, error) {
	return payment.ParseNotify(ctx, profile, r)
}
