package rpc_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"testing"

	"connectrpc.com/connect"

	kitsettings "github.com/imbytecat/moonbase/integrations/core/settings"
	paymentv1 "github.com/imbytecat/moonbase/server/internal/gen/payment/v1"
	"github.com/imbytecat/moonbase/server/internal/pay"
	"github.com/imbytecat/moonbase/server/internal/repository"
	"github.com/imbytecat/moonbase/server/internal/rpc"
	"github.com/imbytecat/moonbase/server/internal/settings"
)

type checkoutGateway struct {
	profile kitsettings.GenericProfile
}

func (checkoutGateway) Describe(provider string) (pay.ProviderDescriptor, bool) {
	return pay.NewRegistry().Describe(provider)
}
func (checkoutGateway) ProfileProducts(profile kitsettings.GenericProfile) []string {
	return pay.NewRegistry().ConfiguredProducts(profile.Provider, profile.Config)
}
func (checkoutGateway) RenderHostedFlow(provider, product, payload string) ([]byte, error) {
	return pay.NewRegistry().RenderHostedFlow(provider, product, payload)
}

func (g checkoutGateway) ProfilesFor(context.Context, string) ([]kitsettings.GenericProfile, error) {
	return []kitsettings.GenericProfile{g.profile}, nil
}
func (g checkoutGateway) ProfileByID(context.Context, string) (kitsettings.GenericProfile, error) {
	return g.profile, nil
}
func (g checkoutGateway) Plan(context.Context, kitsettings.GenericProfile, pay.PlanRequest) (pay.PlanResult, error) {
	descriptor, _ := g.Describe("wechat")
	return pay.PlanResult{ProductID: "native", Input: descriptor.Products[0].Input}, nil
}
func (checkoutGateway) Create(context.Context, kitsettings.GenericProfile, pay.CreateRequest) (pay.Action, error) {
	return pay.Action{QR: &pay.QRAction{Data: "weixin://test"}}, nil
}
func (checkoutGateway) Query(context.Context, kitsettings.GenericProfile, string) (pay.QueryResult, error) {
	return pay.QueryResult{Exists: true, State: pay.StatePending}, nil
}
func (checkoutGateway) Refund(context.Context, kitsettings.GenericProfile, pay.RefundRequest) (pay.RefundResult, error) {
	return pay.RefundResult{}, nil
}
func (checkoutGateway) QueryRefund(context.Context, kitsettings.GenericProfile, string) (bool, error) {
	return false, nil
}
func (checkoutGateway) ParseNotify(context.Context, kitsettings.GenericProfile, *http.Request) (pay.NotifyResult, error) {
	return pay.NotifyResult{}, nil
}

func TestConcurrentCheckoutConfirmationCreatesOneOrder(t *testing.T) {
	_, _, pool := newStackWithPool(t)
	repo := repository.New(pool)
	store := settings.NewStore(repo)
	manager := pay.NewCheckoutIssuer(repo, store, "")
	gateway := checkoutGateway{profile: kitsettings.GenericProfile{
		Id: "wechat-test", Name: "测试微信支付", Provider: "wechat",
		Config: map[string]any{"products": []string{"native"}},
	}}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := rpc.NewPaymentCheckoutService(repo, gateway, manager, logger)
	issued, err := manager.Create(t.Context(), pay.CheckoutCommand{
		Purpose: pay.PurposeCheckout, BusinessReference: "concurrent-order",
		IdempotencyKey: t.Name(), Subject: "并发确认", Amount: 100, ReturnPath: "/payments",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.PlanCheckout(t.Context(), connect.NewRequest(&paymentv1.PlanCheckoutRequest{
		Session: issued.Token, PaymentMethod: "wechat",
	})); err != nil {
		t.Fatal(err)
	}
	read, err := svc.GetCheckoutSession(t.Context(), connect.NewRequest(&paymentv1.GetCheckoutSessionRequest{
		Session: issued.Token,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if got := read.Msg.GetSession().GetPaymentMethod(); got != "wechat" {
		t.Fatalf("planned checkout payment method = %q, want wechat", got)
	}

	const workers = 8
	ids := make(chan string, workers)
	errs := make(chan error, workers)
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := svc.ConfirmCheckout(context.Background(), connect.NewRequest(&paymentv1.ConfirmCheckoutRequest{
				Session: issued.Token, PaymentMethod: "wechat",
			}))
			if err != nil {
				errs <- err
				return
			}
			ids <- resp.Msg.GetOrder().GetOrder().GetId()
		}()
	}
	wg.Wait()
	close(ids)
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	var first string
	for id := range ids {
		if first == "" {
			first = id
		}
		if id != first {
			t.Fatalf("concurrent confirmation returned order %q and %q", first, id)
		}
	}
}
