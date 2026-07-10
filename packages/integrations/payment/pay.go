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

func CreateTyped[T any](
	ctx context.Context,
	config T,
	products []string,
	descriptor ProviderDescriptor,
	request CreateRequest,
	create func(context.Context, T, CreateRequest, string) (string, error),
) (Action, error) {
	product := productByID(descriptor.Products, request.ProductID)
	if product == nil {
		return Action{}, fmt.Errorf("%w: %q", ErrUnknownMethod, request.ProductID)
	}
	if len(products) > 0 && !slices.Contains(products, request.ProductID) {
		return Action{}, fmt.Errorf("%w: %q", ErrMethodNotOffered, request.ProductID)
	}
	if len(product.Input.Fields) > 0 {
		if err := product.Input.Validate(request.Inputs); err != nil {
			return Action{}, fmt.Errorf("%w: %w", ErrMissingInput, err)
		}
	}
	payload, err := create(ctx, config, request, request.NotifyURL)
	if err != nil {
		return Action{}, err
	}
	return actionFor(request.ProductID, payload), nil
}

func InputString(inputs map[string]any, key string) string {
	value, _ := inputs[key].(string)
	return value
}

// yuan renders integer cents as the decimal-yuan string Alipay expects.
func Yuan(cents int64) string {
	return fmt.Sprintf("%d.%02d", cents/100, cents%100)
}
