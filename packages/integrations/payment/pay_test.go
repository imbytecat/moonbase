package pay_test

import (
	"context"
	"errors"
	"maps"
	"slices"
	"testing"

	kitsettings "github.com/imbytecat/moonbase/integrations/core/settings"
	. "github.com/imbytecat/moonbase/integrations/payment"
	alipayprovider "github.com/imbytecat/moonbase/integrations/payment/alipay"
	wechatprovider "github.com/imbytecat/moonbase/integrations/payment/wechat"
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
		if got := Yuan(tc.cents); got != tc.want {
			t.Errorf("yuan(%d) = %q, want %q", tc.cents, got, tc.want)
		}
	}
}

func testRegistry() Registry {
	return MustRegistry(alipayprovider.New(), wechatprovider.New())
}

func TestAlipayUsable(t *testing.T) {
	registry := testRegistry()
	base := kitsettings.GenericProfile{
		Provider: "alipay",
		Config:   map[string]any{"appId": "2021000000000000", "appPrivateKey": "key"},
	}

	publicKey := base
	publicKey.Config = cloneConfig(base.Config)
	publicKey.Config["authMethod"] = AuthPublicKey
	publicKey.Config["alipayPublicKey"] = "pub"
	if !registry.ConfigUsable(publicKey.Provider, publicKey.Config) {
		t.Error("public-key profile with platform key should be usable")
	}

	defaulted := base
	defaulted.Config = cloneConfig(base.Config)
	defaulted.Config["alipayPublicKey"] = "pub"
	if registry.ConfigUsable(defaulted.Provider, defaulted.Config) {
		t.Error("auth_method must be explicit; runtime config has no implicit default")
	}

	missingKey := base
	missingKey.Config = cloneConfig(base.Config)
	missingKey.Config["authMethod"] = AuthPublicKey
	if registry.ConfigUsable(missingKey.Provider, missingKey.Config) {
		t.Error("public-key profile without platform key should not be usable")
	}

	cert := base
	cert.Config = cloneConfig(base.Config)
	cert.Config["authMethod"] = AuthCert
	cert.Config["appCert"] = "a"
	cert.Config["alipayRootCert"] = "b"
	cert.Config["alipayPublicCert"] = "c"
	if !registry.ConfigUsable(cert.Provider, cert.Config) {
		t.Error("cert profile with all three certs should be usable")
	}
	cert.Config["alipayRootCert"] = ""
	if registry.ConfigUsable(cert.Provider, cert.Config) {
		t.Error("cert profile missing a cert should not be usable")
	}
}

func TestWechatUsable(t *testing.T) {
	registry := testRegistry()
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
	if !registry.ConfigUsable(publicKey.Provider, publicKey.Config) {
		t.Error("public-key profile with key id + key should be usable")
	}

	missing := base
	missing.Config = cloneConfig(base.Config)
	missing.Config["authMethod"] = AuthPublicKey
	if registry.ConfigUsable(missing.Provider, missing.Config) {
		t.Error("public-key profile without wechat public key should not be usable")
	}

	platformCert := base
	platformCert.Config = cloneConfig(base.Config)
	platformCert.Config["authMethod"] = AuthPlatformCert
	if !registry.ConfigUsable(platformCert.Provider, platformCert.Config) {
		t.Error("platform-cert profile needs no local platform key")
	}

	noAPIKey := platformCert
	noAPIKey.Config = cloneConfig(platformCert.Config)
	noAPIKey.Config["apiV3Key"] = ""
	if registry.ConfigUsable(noAPIKey.Provider, noAPIKey.Config) {
		t.Error("profile without APIv3 key should not be usable")
	}
}

func TestDriverDescribeAndPlanOwnPaymentProducts(t *testing.T) {
	registry := testRegistry()
	descriptor, ok := registry.Describe("wechat")
	if !ok {
		t.Fatal("wechat driver descriptor not found")
	}
	if len(descriptor.Methods) != 1 || descriptor.Methods[0].Key != "wechat" || descriptor.Methods[0].Presentation.Name != "微信支付" {
		t.Fatalf("methods = %+v, want payer-facing WeChat method", descriptor.Methods)
	}
	if len(descriptor.Products) != 4 || descriptor.Products[0].ID != "native" || descriptor.Products[0].Method != "wechat" {
		t.Fatalf("products = %+v, want driver-owned product catalog", descriptor.Products)
	}
	wantCapabilities := []string{"notify", "refund", "refund_query", "hosted_flow"}
	if !slices.Equal(descriptor.Capabilities, wantCapabilities) {
		t.Fatalf("capabilities = %v, want %v derived from driver interfaces", descriptor.Capabilities, wantCapabilities)
	}

	profile := usableWechat("native", "h5", "jsapi")
	cases := []struct {
		name      string
		userAgent string
		want      string
	}{
		{name: "desktop", userAgent: "Mozilla/5.0 (X11; Linux x86_64)", want: "native"},
		{name: "mobile browser", userAgent: "Mozilla/5.0 (Linux; Android 15) Mobile", want: "h5"},
		{name: "wechat browser", userAgent: "Mozilla/5.0 MicroMessenger/8.0 Mobile", want: "jsapi"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			plan, err := registry.Plan(t.Context(), profile.Provider, profile.Config, PlanRequest{
				PaymentMethod: "wechat",
				Client:        ClientContext{UserAgent: tc.userAgent},
			})
			if err != nil {
				t.Fatal(err)
			}
			if plan.ProductID != tc.want {
				t.Fatalf("product = %q, want %q", plan.ProductID, tc.want)
			}
		})
	}
}

func usableWechat(products ...string) kitsettings.GenericProfile {
	return kitsettings.GenericProfile{
		Provider: "wechat",
		Config: map[string]any{
			"mchId":           "1900000000",
			"appId":           "wx0000000000000000",
			"mchCertSerialNo": "SN",
			"mchPrivateKey":   "key",
			"apiV3Key":        "0123456789abcdef0123456789abcdef",
			"authMethod":      AuthPlatformCert,
			"products":        products,
		},
	}
}

func TestProfileProducts(t *testing.T) {
	registry := testRegistry()
	all := registry.ConfiguredProducts("alipay", map[string]any{})
	want := []string{
		"precreate", "page_pay", "wap_pay", "create", "app_pay",
	}
	if !slices.Equal(all, want) {
		t.Errorf("empty products should offer the whole alipay catalog, got %v", all)
	}

	sub := registry.ConfiguredProducts("alipay", map[string]any{"products": []string{"wap_pay", "native", "precreate"}})
	if !slices.Equal(sub, []string{"precreate", "wap_pay"}) {
		t.Errorf("ProfileProducts should keep signed ids in driver order and drop foreign ids, got %v", sub)
	}
}

// Create validates the method against the catalog before any provider round-trip
// (the wire no longer carries an `in:` list). A method no driver knows is an
// unknown-method error (InvalidArgument at the RPC); a known method the profile
// didn't sign for is a not-offered error (FailedPrecondition) — the two error
// codes the removed `in:` rule and the Offered check used to produce separately.
// The profile is usable because ProfileFor only ever hands Create a usable one.
func TestCreateRejectsUnknownMethod(t *testing.T) {
	profile := usableAlipay("precreate")
	if _, err := testRegistry().Create(context.Background(), profile.Provider, profile.Config, CreateRequest{ProductID: "bogus"}); !errors.Is(err, ErrUnknownMethod) {
		t.Errorf("Create with catalog-unknown method = %v, want ErrUnknownMethod", err)
	}
}

func TestCreateRejectsUnofferedMethod(t *testing.T) {
	profile := usableAlipay("precreate")
	if _, err := testRegistry().Create(context.Background(), profile.Provider, profile.Config, CreateRequest{ProductID: "page_pay"}); !errors.Is(err, ErrMethodNotOffered) {
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
			"products":        methods,
		},
	}
}

func cloneConfig(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	maps.Copy(out, in)
	return out
}
