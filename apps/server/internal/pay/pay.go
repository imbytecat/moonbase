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
	Describe(provider string) (ProviderDescriptor, bool)
	ProfileProducts(profile kitsettings.GenericProfile) []string
	RenderHostedFlow(provider, product, payload string) ([]byte, error)
	ProfilesFor(ctx context.Context, purpose string) ([]kitsettings.GenericProfile, error)
	ProfileByID(ctx context.Context, profileID string) (kitsettings.GenericProfile, error)
	Plan(ctx context.Context, profile kitsettings.GenericProfile, req PlanRequest) (PlanResult, error)
	Create(ctx context.Context, profile kitsettings.GenericProfile, req CreateRequest) (Action, error)
	Query(ctx context.Context, profile kitsettings.GenericProfile, outTradeNo string) (QueryResult, error)
	Refund(ctx context.Context, profile kitsettings.GenericProfile, req RefundRequest) (RefundResult, error)
	QueryRefund(ctx context.Context, profile kitsettings.GenericProfile, refundNo string) (settled bool, err error)
	ParseNotify(ctx context.Context, profile kitsettings.GenericProfile, r *http.Request) (NotifyResult, error)
}

func Currency(provider string) string { return payment.Currency(provider) }

type Client struct {
	store     *settings.Store
	publicURL string
	registry  payment.Registry
}

func NewClient(store *settings.Store, publicURL string, registry payment.Registry) *Client {
	return &Client{store: store, publicURL: strings.TrimSuffix(publicURL, "/"), registry: registry}
}

var _ Gateway = (*Client)(nil)

func (c *Client) notifyURL(profile kitsettings.GenericProfile) string {
	return c.publicURL + "/api/payment/notify/" + profile.Provider + "/" + profile.Id
}

func (c *Client) Describe(provider string) (ProviderDescriptor, bool) {
	return c.registry.Describe(provider)
}

func (c *Client) ProfileProducts(profile kitsettings.GenericProfile) []string {
	return c.registry.ConfiguredProducts(profile.Provider, profile.Config)
}

func (c *Client) RenderHostedFlow(provider, product, payload string) ([]byte, error) {
	return c.registry.RenderHostedFlow(provider, product, payload)
}

func (c *Client) ProfilesFor(ctx context.Context, purpose string) ([]kitsettings.GenericProfile, error) {
	cfg, err := c.store.Payment(ctx)
	if err != nil {
		return nil, err
	}
	bound := cfg.ProfilesFor(purpose)
	out := make([]kitsettings.GenericProfile, 0, len(bound))
	for _, profile := range bound {
		if c.registry.ConfigUsable(profile.Provider, profile.Config) {
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
	if profile, ok := cfg.Profile(profileID); ok && c.registry.ConfigUsable(profile.Provider, profile.Config) {
		return profile, nil
	}
	return kitsettings.GenericProfile{}, ErrNotConfigured
}

func (c *Client) Plan(ctx context.Context, profile kitsettings.GenericProfile, req PlanRequest) (PlanResult, error) {
	return c.registry.Plan(ctx, profile.Provider, profile.Config, req)
}

func (c *Client) Create(ctx context.Context, profile kitsettings.GenericProfile, req CreateRequest) (Action, error) {
	req.NotifyURL = c.notifyURL(profile)
	return c.registry.Create(ctx, profile.Provider, profile.Config, req)
}

func (c *Client) Query(ctx context.Context, profile kitsettings.GenericProfile, outTradeNo string) (QueryResult, error) {
	return c.registry.Query(ctx, profile.Provider, profile.Config, outTradeNo)
}

func (c *Client) Refund(ctx context.Context, profile kitsettings.GenericProfile, req RefundRequest) (RefundResult, error) {
	req.NotifyURL = c.notifyURL(profile)
	return c.registry.Refund(ctx, profile.Provider, profile.Config, req)
}

func (c *Client) QueryRefund(ctx context.Context, profile kitsettings.GenericProfile, refundNo string) (bool, error) {
	return c.registry.QueryRefund(ctx, profile.Provider, profile.Config, refundNo)
}

func (c *Client) ParseNotify(ctx context.Context, profile kitsettings.GenericProfile, r *http.Request) (NotifyResult, error) {
	return c.registry.ParseNotify(ctx, profile.Provider, profile.Config, r)
}
