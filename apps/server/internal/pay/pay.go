package pay

import (
	"context"
	"net/http"
	"strings"

	"github.com/imbytecat/moonbase/packages/integrations/core/integration"
	"github.com/imbytecat/moonbase/packages/integrations/core/schema"
	kitsettings "github.com/imbytecat/moonbase/packages/integrations/core/settings"
	payment "github.com/imbytecat/moonbase/packages/integrations/payment"
	"github.com/imbytecat/moonbase/server/internal/settings"
)

const PurposeCheckout = payment.PurposeCheckout

var Purposes = integration.Catalog{PurposeCheckout}

type CredentialKind = payment.CredentialKind

const (
	CredentialQR       = payment.CredentialQR
	CredentialRedirect = payment.CredentialRedirect
	CredentialParams   = payment.CredentialParams
)

type Input = payment.Input

const (
	InputPayerID   = payment.InputPayerID
	InputReturnURL = payment.InputReturnURL
)

type Method = payment.Method

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

type CreateRequest = payment.CreateRequest
type Credential = payment.Credential
type QueryResult = payment.QueryResult
type RefundRequest = payment.RefundRequest
type RefundResult = payment.RefundResult
type NotifyResult = payment.NotifyResult
type Option = payment.Option

type Gateway interface {
	OptionsFor(ctx context.Context, purpose string) ([]Option, error)
	ProfileFor(ctx context.Context, purpose, profileID string) (kitsettings.GenericProfile, error)
	ProfileByID(ctx context.Context, profileID string) (kitsettings.GenericProfile, error)
	Create(ctx context.Context, p kitsettings.GenericProfile, req CreateRequest) (Credential, error)
	Query(ctx context.Context, p kitsettings.GenericProfile, outTradeNo string) (QueryResult, error)
	Refund(ctx context.Context, p kitsettings.GenericProfile, req RefundRequest) (RefundResult, error)
	QueryRefund(ctx context.Context, p kitsettings.GenericProfile, refundNo string) (settled bool, err error)
	ParseNotify(ctx context.Context, p kitsettings.GenericProfile, r *http.Request) (NotifyResult, error)
}

func Schemas() map[string]schema.Schema { return payment.Schemas() }

func Providers() []string { return payment.Providers() }

func ProfileUsable(p kitsettings.GenericProfile) bool { return payment.ProfileUsable(p) }

func Catalog(provider string) []Method { return payment.Catalog(provider) }

func Methods() []string { return payment.Methods() }

func Offered(p kitsettings.GenericProfile) []string { return payment.Offered(p) }

func ProfileMethods(p kitsettings.GenericProfile) []string { return payment.ProfileMethods(p) }

func ValidateMethods(provider string, methods []string) error {
	return payment.ValidateMethods(provider, methods)
}

func KindOf(provider, method string) CredentialKind { return payment.KindOf(provider, method) }

func InputsOf(provider, method string) []Input { return payment.InputsOf(provider, method) }

func Currency(provider string) string { return payment.Currency(provider) }

type Client struct {
	store     *settings.Store
	publicURL string
}

func NewClient(store *settings.Store, publicURL string) *Client {
	return &Client{store: store, publicURL: strings.TrimSuffix(publicURL, "/")}
}

var _ Gateway = (*Client)(nil)

func (c *Client) notifyURL(p kitsettings.GenericProfile) string {
	return c.publicURL + "/api/payment/notify/" + p.Provider + "/" + p.Id
}

func (c *Client) OptionsFor(ctx context.Context, purpose string) ([]Option, error) {
	cfg, err := c.store.Payment(ctx)
	if err != nil {
		return nil, err
	}
	bound := cfg.ProfilesFor(purpose)
	out := make([]Option, 0, len(bound))
	for _, p := range bound {
		if ProfileUsable(p) {
			out = append(out, Option{ProfileID: p.Id, Name: p.Name, Provider: p.Provider, Methods: Offered(p)})
		}
	}
	return out, nil
}

func (c *Client) ProfileFor(ctx context.Context, purpose, profileID string) (kitsettings.GenericProfile, error) {
	cfg, err := c.store.Payment(ctx)
	if err != nil {
		return kitsettings.GenericProfile{}, err
	}
	for _, p := range cfg.ProfilesFor(purpose) {
		if p.Id == profileID && ProfileUsable(p) {
			return p, nil
		}
	}
	return kitsettings.GenericProfile{}, ErrNotConfigured
}

func (c *Client) ProfileByID(ctx context.Context, profileID string) (kitsettings.GenericProfile, error) {
	cfg, err := c.store.Payment(ctx)
	if err != nil {
		return kitsettings.GenericProfile{}, err
	}
	if p, ok := cfg.Profile(profileID); ok && ProfileUsable(p) {
		return p, nil
	}
	return kitsettings.GenericProfile{}, ErrNotConfigured
}

func (c *Client) Create(ctx context.Context, p kitsettings.GenericProfile, req CreateRequest) (Credential, error) {
	return payment.Create(ctx, p, req, c.notifyURL(p))
}

func (c *Client) Query(ctx context.Context, p kitsettings.GenericProfile, outTradeNo string) (QueryResult, error) {
	return payment.Query(ctx, p, outTradeNo)
}

func (c *Client) Refund(ctx context.Context, p kitsettings.GenericProfile, req RefundRequest) (RefundResult, error) {
	return payment.Refund(ctx, p, req, c.notifyURL(p))
}

func (c *Client) QueryRefund(ctx context.Context, p kitsettings.GenericProfile, refundNo string) (bool, error) {
	return payment.QueryRefund(ctx, p, refundNo)
}

func (c *Client) ParseNotify(ctx context.Context, p kitsettings.GenericProfile, r *http.Request) (NotifyResult, error) {
	return payment.ParseNotify(ctx, p, r)
}
