// Package pay processes payments through gateway connection profiles
// configured in system settings. Each provider is a driver behind the
// Gateway seam (Alipay V3 OpenAPI, WeChat Pay APIv3); the caller passes
// semantic inputs (integer-cent amounts, merchant order numbers, a payment
// METHOD picked per order) and drivers own provider-specific quirks —
// amount formatting, per-method API dialects, trade-state mapping and
// notification signature schemes. A METHOD is one official provider product
// (Alipay API method / WeChat trade_type); each driver declares its product
// CATALOG — every product's credential KIND (qr / redirect / params) and the
// extra INPUTS it collects — and a profile signs for a subset (settings.Methods
// → Offered). The method is a per-ORDER choice constrained to that signed set
// and validated locally (ErrMethodNotOffered / ErrMissingInput) before a
// provider round-trip. Payment purposes are multi-valued: every bound profile
// is a selectable payment option and the payer picks one, so calls are
// profile-addressed after an OptionsFor listing. Clients are built per call so
// config changes apply without a restart.
package pay

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/imbytecat/moonbase/server/internal/channel"
	"github.com/imbytecat/moonbase/server/internal/paymentcatalog"
	"github.com/imbytecat/moonbase/server/internal/settings"
	"github.com/imbytecat/moonbase/server/internal/systemcodec"
)

// PurposeCheckout is the demo checkout slot. Adding a feature that charges =
// adding a purpose here.
const PurposeCheckout = "checkout"

// Purposes is the catalog served to the admin UI, in display order.
var Purposes = channel.Catalog{PurposeCheckout}

// CredentialKind tells the checkout how to consume a created payment.
type CredentialKind string

const (
	// CredentialQR renders the value as a QR code the payer scans.
	CredentialQR CredentialKind = "qr"
	// CredentialRedirect opens the value as a URL — the provider's cashier.
	CredentialRedirect CredentialKind = "redirect"
	// CredentialParams hands the JSON value to a provider SDK (in-app /
	// mini-program invocation) the admin demo can only display.
	CredentialParams CredentialKind = "params"
)

// Input names a per-order field a product needs beyond the common ones.
type Input string

const (
	// InputPayerID is the provider-side payer identity, required by params-kind
	// products (WeChat / Alipay JSAPI) whose invocation is payer-bound.
	InputPayerID Input = "payer_id"
	// InputReturnURL is where a redirect product returns the payer; optional.
	InputReturnURL Input = "return_url"
)

// Method is one official provider product: ID is the provider's official API
// method (Alipay) or trade_type (WeChat), Kind drives client rendering, and
// Inputs are the extra per-order fields it collects.
type Method struct {
	ID     string
	Kind   CredentialKind
	Inputs []Input
}

var (
	ErrNotConfigured    = errors.New("payment is not configured")
	ErrUnknownMethod    = errors.New("payment method is not a known product")
	ErrMethodNotOffered = errors.New("payment method is not offered by this profile")
	ErrMissingInput     = errors.New("payment method is missing a required input")
)

type State int

const (
	StatePending State = iota
	StatePaid
	StateClosed
	StateRefunded
)

type CreateRequest struct {
	OutTradeNo string
	Subject    string
	// Integer cents.
	Amount int64
	Method string
	// Provider-side payer identifier (WeChat openid / Alipay buyer id);
	// required by params-kind products (JSAPI).
	PayerID string
	// Where the provider returns the payer after paying; redirect products
	// (page_pay / wap_pay / h5) only, optional.
	ReturnURL string
	// The payer's client IP; WeChat h5 requires it.
	ClientIP string
}

// Credential is what the checkout client renders, shaped by the method's
// CredentialKind: qr = QR-code content, redirect = a URL to open, params = the
// provider's signed invocation parameters serialized as JSON.
type Credential = string

type QueryResult struct {
	State           State
	ProviderTradeNo string
	PayerID         string
	// Zero when the provider did not report a time; callers default to now.
	PaidAt time.Time
}

type RefundRequest struct {
	OutTradeNo string
	RefundNo   string
	Reason     string
	// Integer cents; full refund.
	Amount int64
}

type RefundResult struct {
	// True when the refund settled synchronously (Alipay); false while the
	// provider processes it asynchronously (WeChat).
	Settled bool
}

type NotifyResult struct {
	OutTradeNo string
	State      State
	Query      QueryResult
	// Ack writes the provider-specific success acknowledgment.
	Ack func(w http.ResponseWriter)
}

type Option struct {
	ProfileID string
	Name      string
	Provider  string
	Methods   []string
}

// Gateway abstracts the provider round-trip for handlers. Orders address a
// concrete profile (picked from OptionsFor) because payment purposes are
// multi-valued.
type Gateway interface {
	OptionsFor(ctx context.Context, purpose string) ([]Option, error)
	ProfileFor(ctx context.Context, purpose, profileID string) (systemcodec.PaymentProfile, error)
	ProfileByID(ctx context.Context, profileID string) (systemcodec.PaymentProfile, error)
	Create(ctx context.Context, p systemcodec.PaymentProfile, req CreateRequest) (Credential, error)
	Query(ctx context.Context, p systemcodec.PaymentProfile, outTradeNo string) (QueryResult, error)
	Refund(ctx context.Context, p systemcodec.PaymentProfile, req RefundRequest) (RefundResult, error)
	QueryRefund(ctx context.Context, p systemcodec.PaymentProfile, refundNo string) (settled bool, err error)
	ParseNotify(ctx context.Context, p systemcodec.PaymentProfile, r *http.Request) (NotifyResult, error)
}

type payOps struct {
	catalog     []Method
	currency    string
	create      func(ctx context.Context, p systemcodec.PaymentProfile, req CreateRequest, notifyURL string) (Credential, error)
	query       func(ctx context.Context, p systemcodec.PaymentProfile, outTradeNo string) (QueryResult, error)
	refund      func(ctx context.Context, p systemcodec.PaymentProfile, req RefundRequest, notifyURL string) (RefundResult, error)
	queryRefund func(ctx context.Context, p systemcodec.PaymentProfile, refundNo string) (bool, error)
	parseNotify func(ctx context.Context, p systemcodec.PaymentProfile, r *http.Request) (NotifyResult, error)
}

var drivers = channel.Registry[systemcodec.PaymentProfile, payOps]{
	"alipay": {
		Usable: alipayUsable,
		Ops: payOps{
			catalog:     methodCatalog("alipay"),
			currency:    "CNY",
			create:      alipayCreate,
			query:       alipayQuery,
			refund:      alipayRefund,
			queryRefund: alipayQueryRefund,
			parseNotify: alipayParseNotify,
		},
	},
	"wechat": {
		Usable: wechatUsable,
		Ops: payOps{
			catalog:     methodCatalog("wechat"),
			currency:    "CNY",
			create:      wechatCreate,
			query:       wechatQuery,
			refund:      wechatRefund,
			queryRefund: wechatQueryRefund,
			parseNotify: wechatParseNotify,
		},
	},
}

// methodCatalog builds a provider's product list from the generated catalog
// (paymentcatalog.Methods, derived from the payment.v1.PaymentMethod enum),
// mapping the stringly-typed generated data to the pay domain types. The
// generated slice is in enum-declaration order, so each provider keeps its
// checkout display order.
func methodCatalog(provider string) []Method {
	var out []Method
	for _, m := range paymentcatalog.Methods {
		if m.Provider != provider {
			continue
		}
		out = append(out, Method{
			ID:     m.ID,
			Kind:   CredentialKind(m.Kind),
			Inputs: toInputs(m.Inputs),
		})
	}
	return out
}

func toInputs(names []string) []Input {
	if len(names) == 0 {
		return nil
	}
	out := make([]Input, len(names))
	for i, n := range names {
		out[i] = Input(n)
	}
	return out
}

// Providers lists registered driver names, sorted.
func Providers() []string {
	return drivers.Names()
}

// ProfileUsable reports whether the profile's driver is fully configured —
// the same gate every Gateway call enforces.
func ProfileUsable(p systemcodec.PaymentProfile) bool {
	return drivers.ProfileUsable(p)
}

// Catalog lists a provider's products in display order.
func Catalog(provider string) []Method {
	return drivers[provider].Ops.catalog
}

// Methods lists every product id across all drivers, sorted — the union the
// proto method `in:` constraint mirrors (TestPaymentMethodsMatchContract).
func Methods() []string {
	var out []string
	for _, name := range drivers.Names() {
		for _, m := range drivers[name].Ops.catalog {
			out = append(out, m.ID)
		}
	}
	slices.Sort(out)
	return out
}

// Offered lists the products a profile presents at checkout: those the
// merchant signed for (p.Methods), in the driver's display order. Empty
// p.Methods means "all of the provider's products" so profiles saved before
// per-product selection keep working.
func Offered(p systemcodec.PaymentProfile) []string {
	out := make([]string, 0, len(drivers[p.Provider].Ops.catalog))
	for _, m := range drivers[p.Provider].Ops.catalog {
		if len(p.Methods) == 0 || slices.Contains(p.Methods, m.ID) {
			out = append(out, m.ID)
		}
	}
	return out
}

func methodByID(provider, method string) (Method, bool) {
	for _, m := range drivers[provider].Ops.catalog {
		if m.ID == method {
			return m, true
		}
	}
	return Method{}, false
}

// ValidateMethods reports whether every id in methods is a product of the
// provider's catalog. An empty list is valid — it means "all products". This is
// the save-time guard for a profile's signed products, replacing the removed
// proto `in:` rule on PaymentProfile.methods with a catalog-derived check.
func ValidateMethods(provider string, methods []string) error {
	for _, id := range methods {
		if _, ok := methodByID(provider, id); !ok {
			return fmt.Errorf("%w: %q for provider %q", ErrUnknownMethod, id, provider)
		}
	}
	return nil
}

// KindOf reports how an order's credential should be consumed by the client.
func KindOf(provider, method string) CredentialKind {
	m, _ := methodByID(provider, method)
	return m.Kind
}

// InputsOf lists the extra fields a product collects at checkout.
func InputsOf(provider, method string) []Input {
	m, _ := methodByID(provider, method)
	return m.Inputs
}

// Currency is the settlement currency of a provider's orders. The system is
// CNY-only (see docs/adr/0001): both CN drivers settle in CNY, and this exists
// only so the payment_orders.currency column carries an honest value — not as a
// multi-currency seam.
func Currency(provider string) string {
	if c := drivers[provider].Ops.currency; c != "" {
		return c
	}
	return "CNY"
}

type Client struct {
	store *settings.Store
	// publicURL is the externally reachable server base; async notification
	// URLs are built from it.
	publicURL string
}

func NewClient(store *settings.Store, publicURL string) *Client {
	return &Client{store: store, publicURL: strings.TrimSuffix(publicURL, "/")}
}

var _ Gateway = (*Client)(nil)

func (c *Client) notifyURL(p systemcodec.PaymentProfile) string {
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

func (c *Client) ProfileFor(ctx context.Context, purpose, profileID string) (systemcodec.PaymentProfile, error) {
	cfg, err := c.store.Payment(ctx)
	if err != nil {
		return systemcodec.PaymentProfile{}, err
	}
	for _, p := range cfg.ProfilesFor(purpose) {
		if p.Id == profileID && ProfileUsable(p) {
			return p, nil
		}
	}
	return systemcodec.PaymentProfile{}, ErrNotConfigured
}

func (c *Client) ProfileByID(ctx context.Context, profileID string) (systemcodec.PaymentProfile, error) {
	cfg, err := c.store.Payment(ctx)
	if err != nil {
		return systemcodec.PaymentProfile{}, err
	}
	if p, ok := cfg.Profile(profileID); ok && ProfileUsable(p) {
		return p, nil
	}
	return systemcodec.PaymentProfile{}, ErrNotConfigured
}

func (c *Client) Create(ctx context.Context, p systemcodec.PaymentProfile, req CreateRequest) (Credential, error) {
	ops, ok := drivers.OpsFor(p)
	if !ok {
		return "", ErrNotConfigured
	}
	if !slices.Contains(Methods(), req.Method) {
		return "", fmt.Errorf("%w: %q", ErrUnknownMethod, req.Method)
	}
	if !slices.Contains(Offered(p), req.Method) {
		return "", fmt.Errorf("%w: %q", ErrMethodNotOffered, req.Method)
	}
	if slices.Contains(InputsOf(p.Provider, req.Method), InputPayerID) && req.PayerID == "" {
		return "", fmt.Errorf("%w: payer_id for %q", ErrMissingInput, req.Method)
	}
	return ops.create(ctx, p, req, c.notifyURL(p))
}

func (c *Client) Query(ctx context.Context, p systemcodec.PaymentProfile, outTradeNo string) (QueryResult, error) {
	ops, ok := drivers.OpsFor(p)
	if !ok {
		return QueryResult{}, ErrNotConfigured
	}
	return ops.query(ctx, p, outTradeNo)
}

func (c *Client) Refund(ctx context.Context, p systemcodec.PaymentProfile, req RefundRequest) (RefundResult, error) {
	ops, ok := drivers.OpsFor(p)
	if !ok {
		return RefundResult{}, ErrNotConfigured
	}
	return ops.refund(ctx, p, req, c.notifyURL(p))
}

func (c *Client) QueryRefund(ctx context.Context, p systemcodec.PaymentProfile, refundNo string) (bool, error) {
	ops, ok := drivers.OpsFor(p)
	if !ok {
		return false, ErrNotConfigured
	}
	return ops.queryRefund(ctx, p, refundNo)
}

func (c *Client) ParseNotify(ctx context.Context, p systemcodec.PaymentProfile, r *http.Request) (NotifyResult, error) {
	ops, ok := drivers.OpsFor(p)
	if !ok {
		return NotifyResult{}, ErrNotConfigured
	}
	return ops.parseNotify(ctx, p, r)
}

// yuan renders integer cents as the decimal-yuan string Alipay expects.
func yuan(cents int64) string {
	return fmt.Sprintf("%d.%02d", cents/100, cents%100)
}
