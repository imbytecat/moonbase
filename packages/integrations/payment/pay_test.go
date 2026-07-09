package pay

import (
	"context"
	"errors"
	"maps"
	"slices"
	"testing"
	"time"

	"github.com/smartwalle/alipay/v3"

	kitsettings "github.com/imbytecat/moonbase/packages/integrations/core/settings"
)

func TestYuanRendersCents(t *testing.T) {
	cases := []struct {
		cents int64
		want  string
	}{
		{1, "0.01"},
		{100, "1.00"},
		{12345, "123.45"},
		{10000000000, "100000000.00"},
	}
	for _, tc := range cases {
		if got := yuan(tc.cents); got != tc.want {
			t.Errorf("yuan(%d) = %q, want %q", tc.cents, got, tc.want)
		}
	}
}

func TestAlipayUsable(t *testing.T) {
	base := kitsettings.GenericProfile{
		Provider: "alipay",
		Config:   map[string]any{"appId": "2021000000000000", "appPrivateKey": "key"},
	}

	publicKey := base
	publicKey.Config = cloneConfig(base.Config)
	publicKey.Config["authMethod"] = AuthPublicKey
	publicKey.Config["alipayPublicKey"] = "pub"
	if !ProfileUsable(publicKey) {
		t.Error("public-key profile with platform key should be usable")
	}

	defaulted := base
	defaulted.Config = cloneConfig(base.Config)
	defaulted.Config["alipayPublicKey"] = "pub"
	if !ProfileUsable(defaulted) {
		t.Error("empty auth_method should default to public-key mode")
	}

	missingKey := base
	missingKey.Config = cloneConfig(base.Config)
	missingKey.Config["authMethod"] = AuthPublicKey
	if ProfileUsable(missingKey) {
		t.Error("public-key profile without platform key should not be usable")
	}

	cert := base
	cert.Config = cloneConfig(base.Config)
	cert.Config["authMethod"] = AuthCert
	cert.Config["appCert"] = "a"
	cert.Config["alipayRootCert"] = "b"
	cert.Config["alipayPublicCert"] = "c"
	if !ProfileUsable(cert) {
		t.Error("cert profile with all three certs should be usable")
	}
	cert.Config["alipayRootCert"] = ""
	if ProfileUsable(cert) {
		t.Error("cert profile missing a cert should not be usable")
	}
}

func TestWechatUsable(t *testing.T) {
	base := kitsettings.GenericProfile{
		Provider: "wechat",
		Config: map[string]any{
			"mchId":           "1900000000",
			"appId":           "wx0000000000000000",
			"mchCertSerialNo": "SN",
			"mchPrivateKey":   "key",
			"apiV3Key":        "0123456789abcdef0123456789abcdef",
		},
	}

	publicKey := base
	publicKey.Config = cloneConfig(base.Config)
	publicKey.Config["authMethod"] = AuthPublicKey
	publicKey.Config["publicKeyId"] = "PUB_KEY_ID_1"
	publicKey.Config["publicKey"] = "pub"
	if !ProfileUsable(publicKey) {
		t.Error("public-key profile with key id + key should be usable")
	}

	missing := base
	missing.Config = cloneConfig(base.Config)
	missing.Config["authMethod"] = AuthPublicKey
	if ProfileUsable(missing) {
		t.Error("public-key profile without wechat public key should not be usable")
	}

	platformCert := base
	platformCert.Config = cloneConfig(base.Config)
	platformCert.Config["authMethod"] = AuthPlatformCert
	if !ProfileUsable(platformCert) {
		t.Error("platform-cert profile needs no local platform key")
	}

	noAPIKey := platformCert
	noAPIKey.Config = cloneConfig(platformCert.Config)
	noAPIKey.Config["apiV3Key"] = ""
	if ProfileUsable(noAPIKey) {
		t.Error("profile without APIv3 key should not be usable")
	}
}

func TestUnknownProviderNotUsable(t *testing.T) {
	if ProfileUsable(kitsettings.GenericProfile{Provider: "paypal"}) {
		t.Error("unregistered provider should not be usable")
	}
}

func TestAlipayStateMapping(t *testing.T) {
	cases := []struct {
		in   alipay.TradeStatus
		want State
	}{
		{alipay.TradeStatusSuccess, StatePaid},
		{alipay.TradeStatusFinished, StatePaid},
		{alipay.TradeStatusClosed, StateClosed},
		{alipay.TradeStatusWaitBuyerPay, StatePending},
		{"", StatePending},
	}
	for _, tc := range cases {
		if got := alipayState(tc.in); got != tc.want {
			t.Errorf("alipayState(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestWechatTradeStateMapping(t *testing.T) {
	cases := []struct {
		in   string
		want State
	}{
		{"SUCCESS", StatePaid},
		{"CLOSED", StateClosed},
		{"REVOKED", StateClosed},
		{"PAYERROR", StateClosed},
		{"REFUND", StateRefunded},
		{"NOTPAY", StatePending},
		{"USERPAYING", StatePending},
	}
	for _, tc := range cases {
		if got := wechatTradeState(tc.in); got != tc.want {
			t.Errorf("wechatTradeState(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestAlipayTimeParsesBeijingTime(t *testing.T) {
	got := alipayTime("2026-07-11 12:30:00")
	want := time.Date(2026, 7, 11, 12, 30, 0, 0, time.FixedZone("CST", 8*3600))
	if !got.Equal(want) {
		t.Errorf("alipayTime = %v, want %v", got, want)
	}
	if !alipayTime("").IsZero() {
		t.Error("empty timestamp should parse to zero time")
	}
	if !alipayTime("garbage").IsZero() {
		t.Error("malformed timestamp should parse to zero time")
	}
}

func TestOffered(t *testing.T) {
	all := Offered(kitsettings.GenericProfile{Provider: "alipay", Config: map[string]any{}})
	want := []string{
		alipayMethodPreCreate,
		alipayMethodPagePay,
		alipayMethodWapPay,
		alipayMethodCreate,
		alipayMethodAppPay,
	}
	if !slices.Equal(all, want) {
		t.Errorf("empty methods should offer the whole alipay catalog, got %v", all)
	}

	sub := Offered(kitsettings.GenericProfile{
		Provider: "alipay",
		Config:   map[string]any{"methods": []string{alipayMethodWapPay, "native", alipayMethodPreCreate}},
	})
	if !slices.Equal(sub, []string{alipayMethodPreCreate, alipayMethodWapPay}) {
		t.Errorf("Offered should keep signed alipay ids in catalog order and drop foreign ids, got %v", sub)
	}
}

// Create validates the method against the catalog before any provider round-trip
// (the wire no longer carries an `in:` list). A method no driver knows is an
// unknown-method error (InvalidArgument at the RPC); a known method the profile
// didn't sign for is a not-offered error (FailedPrecondition) — the two error
// codes the removed `in:` rule and the Offered check used to produce separately.
// The profile is usable because ProfileFor only ever hands Create a usable one.
func TestCreateRejectsUnknownMethod(t *testing.T) {
	if _, err := Create(context.Background(), usableAlipay("precreate"), CreateRequest{Method: "bogus"}, "http://x"); !errors.Is(err, ErrUnknownMethod) {
		t.Errorf("Create with catalog-unknown method = %v, want ErrUnknownMethod", err)
	}
}

func TestCreateRejectsUnofferedMethod(t *testing.T) {
	if _, err := Create(context.Background(), usableAlipay("precreate"), CreateRequest{Method: "page_pay"}, "http://x"); !errors.Is(err, ErrMethodNotOffered) {
		t.Errorf("Create with known-but-unoffered method = %v, want ErrMethodNotOffered", err)
	}
}

func usableAlipay(methods ...string) kitsettings.GenericProfile {
	return kitsettings.GenericProfile{
		Provider: "alipay",
		Config: map[string]any{
			"appId":           "2021000000000000",
			"appPrivateKey":   "key",
			"authMethod":      AuthPublicKey,
			"alipayPublicKey": "pub",
			"methods":         methods,
		},
	}
}

func cloneConfig(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	maps.Copy(out, in)
	return out
}

// TestCatalog is the behavioral spec for the whole method catalog: each
// provider's products in display order with their credential kind and inputs,
// plus the sorted id union. The catalog is generated from the proto
// PaymentMethod enum (protoc-gen-paymentcatalog); this pins the generated data
// to the established set so drivers, Offered/KindOf/InputsOf, and the checkout
// keep behaving identically.
func TestCatalog(t *testing.T) {
	alipay := []Method{
		{ID: "precreate", Kind: CredentialQR},
		{ID: "page_pay", Kind: CredentialRedirect, Inputs: []Input{InputReturnURL}},
		{ID: "wap_pay", Kind: CredentialRedirect, Inputs: []Input{InputReturnURL}},
		{ID: "create", Kind: CredentialParams, Inputs: []Input{InputPayerID}},
		{ID: "app_pay", Kind: CredentialParams},
	}
	assertCatalog(t, "alipay", alipay)

	wechat := []Method{
		{ID: "native", Kind: CredentialQR},
		{ID: "h5", Kind: CredentialRedirect},
		{ID: "jsapi", Kind: CredentialParams, Inputs: []Input{InputPayerID}},
		{ID: "app", Kind: CredentialParams},
	}
	assertCatalog(t, "wechat", wechat)

	wantMethods := []string{"app", "app_pay", "create", "h5", "jsapi", "native", "page_pay", "precreate", "wap_pay"}
	if !slices.Equal(Methods(), wantMethods) {
		t.Errorf("Methods() = %v, want sorted union %v", Methods(), wantMethods)
	}
}

func assertCatalog(t *testing.T, provider string, want []Method) {
	t.Helper()
	got := Catalog(provider)
	if len(got) != len(want) {
		t.Fatalf("%s catalog has %d methods, want %d: %+v", provider, len(got), len(want), got)
	}
	for i, w := range want {
		if got[i].ID != w.ID || got[i].Kind != w.Kind || !slices.Equal(got[i].Inputs, w.Inputs) {
			t.Errorf("%s catalog[%d] = %+v, want %+v", provider, i, got[i], w)
		}
	}
}

func TestKindAndInputs(t *testing.T) {
	if KindOf("alipay", alipayMethodPreCreate) != CredentialQR {
		t.Error("precreate should be a QR credential")
	}
	if KindOf("alipay", alipayMethodPagePay) != CredentialRedirect {
		t.Error("page_pay should be a redirect credential")
	}
	if KindOf("alipay", alipayMethodAppPay) != CredentialParams {
		t.Error("app_pay should be a params credential")
	}
	if !slices.Contains(InputsOf("alipay", alipayMethodCreate), InputPayerID) {
		t.Error("alipay create should collect payer_id")
	}
	if !slices.Contains(InputsOf("alipay", alipayMethodWapPay), InputReturnURL) {
		t.Error("alipay wap_pay should collect return_url")
	}
}
