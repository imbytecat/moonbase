// Package pay contains self-describing payment drivers. Base chooses a
// payer-facing method, the driver plans one provider product from the client
// environment, and Create returns a provider-independent typed action.
package pay

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"time"

	"github.com/imbytecat/moonbase/integrations/core/integration"
	kitsettings "github.com/imbytecat/moonbase/integrations/core/settings"
)

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
	Amount    int64
	ProductID string
	Inputs    map[string]any
	// ReturnURL is server-selected from the checkout session's validated
	// same-origin return path; clients cannot override it.
	ReturnURL string
	// NotifyURL is selected by base from the configured public origin.
	NotifyURL string
	Client    ClientContext
}

type Action struct {
	QR         *QRAction         `json:"qr,omitempty"`
	Redirect   *RedirectAction   `json:"redirect,omitempty"`
	Form       *FormAction       `json:"form,omitempty"`
	Wait       *WaitAction       `json:"wait,omitempty"`
	HostedFlow *HostedFlowAction `json:"hosted_flow,omitempty"`
}

type QRAction struct {
	Data      string    `json:"data"`
	ExpiresAt time.Time `json:"expires_at,omitempty"`
}

type RedirectAction struct {
	URL string `json:"url"`
}

type FormAction struct {
	URL    string            `json:"url"`
	Method string            `json:"method"`
	Fields map[string]string `json:"fields"`
}

type WaitAction struct {
	PollAfterMS int32 `json:"poll_after_ms"`
}

// Payload is stored server-side and served only through the signed hosted-flow
// HTTP seam. It is never returned directly to the business page.
type HostedFlowAction struct {
	Payload string `json:"payload"`
}

type QueryResult struct {
	Exists          bool
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
	Amount    int64
	NotifyURL string
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

// Driver is the mandatory payment provider seam. Optional operations are
// discovered through the capability interfaces below.
type Driver interface {
	Describe() ProviderDescriptor
	Plan(context.Context, kitsettings.GenericProfile, PlanRequest) (PlanResult, error)
	Create(context.Context, kitsettings.GenericProfile, CreateRequest) (Action, error)
	Query(context.Context, kitsettings.GenericProfile, string) (QueryResult, error)
}

type NotifyDriver interface {
	ParseNotify(context.Context, kitsettings.GenericProfile, *http.Request) (NotifyResult, error)
}

type RefundDriver interface {
	Refund(context.Context, kitsettings.GenericProfile, RefundRequest) (RefundResult, error)
}

type RefundQueryDriver interface {
	QueryRefund(context.Context, kitsettings.GenericProfile, string) (bool, error)
}

type HostedFlowDriver interface {
	RenderHostedFlow(product, payload string) ([]byte, error)
}

type ActionRecoveryDriver interface {
	RecoverAction(context.Context, kitsettings.GenericProfile, string) (Action, error)
}

type coreDriver struct {
	descriptor ProviderDescriptor
	create     func(context.Context, kitsettings.GenericProfile, CreateRequest, string) (string, error)
	query      func(context.Context, kitsettings.GenericProfile, string) (QueryResult, error)
}

func (d coreDriver) Describe() ProviderDescriptor { return d.descriptor }

func (d coreDriver) Plan(_ context.Context, profile kitsettings.GenericProfile, req PlanRequest) (PlanResult, error) {
	return plan(d.descriptor, profile, req)
}

func (d coreDriver) Create(ctx context.Context, profile kitsettings.GenericProfile, req CreateRequest) (Action, error) {
	product := productByID(d.descriptor.Products, req.ProductID)
	if product == nil {
		return Action{}, fmt.Errorf("%w: %q", ErrUnknownMethod, req.ProductID)
	}
	if !slices.Contains(ProfileProducts(profile), req.ProductID) {
		return Action{}, fmt.Errorf("%w: %q", ErrMethodNotOffered, req.ProductID)
	}
	if len(product.Input.Fields) > 0 {
		if err := product.Input.Validate(req.Inputs); err != nil {
			return Action{}, fmt.Errorf("%w: %w", ErrMissingInput, err)
		}
	}
	payload, err := d.create(ctx, profile, req, req.NotifyURL)
	if err != nil {
		return Action{}, err
	}
	return actionFor(req.ProductID, payload), nil
}

func (d coreDriver) Query(ctx context.Context, profile kitsettings.GenericProfile, outTradeNo string) (QueryResult, error) {
	return d.query(ctx, profile, outTradeNo)
}

type providerDriver struct {
	coreDriver
	provider    string
	refund      func(context.Context, kitsettings.GenericProfile, RefundRequest, string) (RefundResult, error)
	queryRefund func(context.Context, kitsettings.GenericProfile, string) (bool, error)
	parseNotify func(context.Context, kitsettings.GenericProfile, *http.Request) (NotifyResult, error)
}

func (d providerDriver) Refund(ctx context.Context, profile kitsettings.GenericProfile, req RefundRequest) (RefundResult, error) {
	return d.refund(ctx, profile, req, req.NotifyURL)
}

func (d providerDriver) QueryRefund(ctx context.Context, profile kitsettings.GenericProfile, refundNo string) (bool, error) {
	return d.queryRefund(ctx, profile, refundNo)
}

func (d providerDriver) ParseNotify(ctx context.Context, profile kitsettings.GenericProfile, request *http.Request) (NotifyResult, error) {
	return d.parseNotify(ctx, profile, request)
}

func (d providerDriver) RenderHostedFlow(product, payload string) ([]byte, error) {
	return renderHostedFlow(d.provider, product, payload)
}

var Registry = integration.MustRegistry([]integration.Entry[Driver]{
	{
		Key:          "alipay",
		Presentation: integration.Presentation{Name: "支付宝", Description: "支付宝开放平台直连商户", Color: "#1677ff", IconRef: "antd:AlipayCircleOutlined"},
		Config:       alipaySchema,
		Ops: providerDriver{
			provider:    "alipay",
			coreDriver:  coreDriver{descriptor: alipayDescriptor, create: alipayCreate, query: alipayQuery},
			refund:      alipayRefund,
			queryRefund: alipayQueryRefund,
			parseNotify: alipayParseNotify,
		},
	},
	{
		Key:          "wechat",
		Presentation: integration.Presentation{Name: "微信支付", Description: "微信支付直连商户", Color: "#07c160", IconRef: "antd:WechatOutlined"},
		Config:       wechatSchema,
		Ops: providerDriver{
			provider:    "wechat",
			coreDriver:  coreDriver{descriptor: wechatDescriptor, create: wechatCreate, query: wechatQuery},
			refund:      wechatRefund,
			queryRefund: wechatQueryRefund,
			parseNotify: wechatParseNotify,
		},
	},
})

var (
	_ Driver            = providerDriver{}
	_ NotifyDriver      = providerDriver{}
	_ RefundDriver      = providerDriver{}
	_ RefundQueryDriver = providerDriver{}
	_ HostedFlowDriver  = providerDriver{}
)

func driverCapabilities(driver Driver) []string {
	var capabilities []string
	if _, ok := driver.(NotifyDriver); ok {
		capabilities = append(capabilities, "notify")
	}
	if _, ok := driver.(RefundDriver); ok {
		capabilities = append(capabilities, "refund")
	}
	if _, ok := driver.(RefundQueryDriver); ok {
		capabilities = append(capabilities, "refund_query")
	}
	if _, ok := driver.(HostedFlowDriver); ok {
		capabilities = append(capabilities, "hosted_flow")
	}
	if _, ok := driver.(ActionRecoveryDriver); ok {
		capabilities = append(capabilities, "action_recovery")
	}
	return capabilities
}

func Create(ctx context.Context, p kitsettings.GenericProfile, req CreateRequest, notifyURL string) (Action, error) {
	driver, ok := Registry.OpsFor(p.Provider, p.Config)
	if !ok {
		return Action{}, ErrNotConfigured
	}
	req.NotifyURL = notifyURL
	return driver.Create(ctx, p, req)
}

func actionFor(productID, payload string) Action {
	switch productID {
	case "precreate", "native":
		return Action{QR: &QRAction{Data: payload, ExpiresAt: time.Now().Add(15 * time.Minute)}}
	case "page_pay", "wap_pay", "h5":
		return Action{Redirect: &RedirectAction{URL: payload}}
	case "create", "app_pay", "jsapi", "app":
		return Action{HostedFlow: &HostedFlowAction{Payload: payload}}
	default:
		return Action{Wait: &WaitAction{PollAfterMS: 2000}}
	}
}

func inputString(inputs map[string]any, key string) string {
	value, _ := inputs[key].(string)
	return value
}

func Query(ctx context.Context, p kitsettings.GenericProfile, outTradeNo string) (QueryResult, error) {
	driver, ok := Registry.OpsFor(p.Provider, p.Config)
	if !ok {
		return QueryResult{}, ErrNotConfigured
	}
	return driver.Query(ctx, p, outTradeNo)
}

func Refund(ctx context.Context, p kitsettings.GenericProfile, req RefundRequest, notifyURL string) (RefundResult, error) {
	driver, ok := Registry.OpsFor(p.Provider, p.Config)
	if !ok {
		return RefundResult{}, ErrNotConfigured
	}
	refunder, ok := driver.(RefundDriver)
	if !ok {
		return RefundResult{}, ErrNotConfigured
	}
	req.NotifyURL = notifyURL
	return refunder.Refund(ctx, p, req)
}

func QueryRefund(ctx context.Context, p kitsettings.GenericProfile, refundNo string) (bool, error) {
	driver, ok := Registry.OpsFor(p.Provider, p.Config)
	if !ok {
		return false, ErrNotConfigured
	}
	querier, ok := driver.(RefundQueryDriver)
	if !ok {
		return false, ErrNotConfigured
	}
	return querier.QueryRefund(ctx, p, refundNo)
}

func ParseNotify(ctx context.Context, p kitsettings.GenericProfile, r *http.Request) (NotifyResult, error) {
	driver, ok := Registry.OpsFor(p.Provider, p.Config)
	if !ok {
		return NotifyResult{}, ErrNotConfigured
	}
	notifier, ok := driver.(NotifyDriver)
	if !ok {
		return NotifyResult{}, ErrNotConfigured
	}
	return notifier.ParseNotify(ctx, p, r)
}

func RenderHostedFlow(provider, product, payload string) ([]byte, error) {
	entry, ok := Registry.EntryFor(provider)
	if !ok {
		return nil, ErrNotConfigured
	}
	renderer, ok := entry.Ops.(HostedFlowDriver)
	if !ok {
		return nil, ErrNotConfigured
	}
	return renderer.RenderHostedFlow(product, payload)
}

func cfgStr(config map[string]any, key string) string {
	s, _ := config[key].(string)
	return s
}

// yuan renders integer cents as the decimal-yuan string Alipay expects.
func yuan(cents int64) string {
	return fmt.Sprintf("%d.%02d", cents/100, cents%100)
}
