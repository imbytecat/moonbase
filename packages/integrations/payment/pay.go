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
	"time"

	"github.com/imbytecat/moonbase/integrations/core/integration"
	"github.com/imbytecat/moonbase/integrations/core/schema"
	kitsettings "github.com/imbytecat/moonbase/integrations/core/settings"
)

// PurposeCheckout is the demo checkout slot. Adding a feature that charges =
// adding a purpose here.
const PurposeCheckout = "checkout"

// Purposes is the catalog served to the admin UI, in display order.
var Purposes = integration.Catalog{PurposeCheckout}

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

const (
	AuthPublicKey    = "public_key"
	AuthCert         = "cert"
	AuthPlatformCert = "platform_cert"
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
	ProfileFor(ctx context.Context, purpose, profileID string) (kitsettings.GenericProfile, error)
	ProfileByID(ctx context.Context, profileID string) (kitsettings.GenericProfile, error)
	Create(ctx context.Context, p kitsettings.GenericProfile, req CreateRequest) (Credential, error)
	Query(ctx context.Context, p kitsettings.GenericProfile, outTradeNo string) (QueryResult, error)
	Refund(ctx context.Context, p kitsettings.GenericProfile, req RefundRequest) (RefundResult, error)
	QueryRefund(ctx context.Context, p kitsettings.GenericProfile, refundNo string) (settled bool, err error)
	ParseNotify(ctx context.Context, p kitsettings.GenericProfile, r *http.Request) (NotifyResult, error)
}

type payOps struct {
	catalog     []Method
	currency    string
	create      func(ctx context.Context, p kitsettings.GenericProfile, req CreateRequest, notifyURL string) (Credential, error)
	query       func(ctx context.Context, p kitsettings.GenericProfile, outTradeNo string) (QueryResult, error)
	refund      func(ctx context.Context, p kitsettings.GenericProfile, req RefundRequest, notifyURL string) (RefundResult, error)
	queryRefund func(ctx context.Context, p kitsettings.GenericProfile, refundNo string) (bool, error)
	parseNotify func(ctx context.Context, p kitsettings.GenericProfile, r *http.Request) (NotifyResult, error)
}

type driver struct {
	schema schema.Schema
	ops    payOps
}

var drivers = map[string]driver{
	"alipay": {
		schema: alipaySchema,
		ops: payOps{
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
		schema: wechatSchema,
		ops: payOps{
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

func Create(ctx context.Context, p kitsettings.GenericProfile, req CreateRequest, notifyURL string) (Credential, error) {
	d, ok := drivers[p.Provider]
	if !ok || !ProfileUsable(p) {
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
	return d.ops.create(ctx, p, req, notifyURL)
}

func Query(ctx context.Context, p kitsettings.GenericProfile, outTradeNo string) (QueryResult, error) {
	d, ok := drivers[p.Provider]
	if !ok || !ProfileUsable(p) {
		return QueryResult{}, ErrNotConfigured
	}
	return d.ops.query(ctx, p, outTradeNo)
}

func Refund(ctx context.Context, p kitsettings.GenericProfile, req RefundRequest, notifyURL string) (RefundResult, error) {
	d, ok := drivers[p.Provider]
	if !ok || !ProfileUsable(p) {
		return RefundResult{}, ErrNotConfigured
	}
	return d.ops.refund(ctx, p, req, notifyURL)
}

func QueryRefund(ctx context.Context, p kitsettings.GenericProfile, refundNo string) (bool, error) {
	d, ok := drivers[p.Provider]
	if !ok || !ProfileUsable(p) {
		return false, ErrNotConfigured
	}
	return d.ops.queryRefund(ctx, p, refundNo)
}

func ParseNotify(ctx context.Context, p kitsettings.GenericProfile, r *http.Request) (NotifyResult, error) {
	d, ok := drivers[p.Provider]
	if !ok || !ProfileUsable(p) {
		return NotifyResult{}, ErrNotConfigured
	}
	return d.ops.parseNotify(ctx, p, r)
}

func cfgStr(config map[string]any, key string) string {
	s, _ := config[key].(string)
	return s
}

// yuan renders integer cents as the decimal-yuan string Alipay expects.
func yuan(cents int64) string {
	return fmt.Sprintf("%d.%02d", cents/100, cents%100)
}
