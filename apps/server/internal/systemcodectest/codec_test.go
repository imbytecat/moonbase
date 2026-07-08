package systemcodectest

import (
	"testing"

	"github.com/imbytecat/moonbase/server/integrationkit/systemcodec"
	systemv1 "github.com/imbytecat/moonbase/server/internal/gen/system/v1"
)

func TestEmailCodecMaskBlanksSecretsAndSetsFlags(t *testing.T) {
	p := systemcodec.EmailProfile{
		Id:         "1",
		Name:       "prod",
		Provider:   "smtp",
		Smtp:       systemcodec.SmtpConfig{Host: "smtp.example.com", Password: "topsecret"},
		Cloudflare: systemcodec.CloudflareEmailConfig{ApiToken: "cf-token"},
	}

	masked := systemcodec.EmailCodec.Mask(p)

	if got := masked.GetSmtp().GetPassword(); got != "" {
		t.Errorf("Mask leaked smtp password over the wire: %q", got)
	}
	if !masked.GetSmtp().GetPasswordSet() {
		t.Error("Mask must set password_set when a secret is stored")
	}
	if got := masked.GetCloudflare().GetApiToken(); got != "" {
		t.Errorf("Mask leaked cloudflare api token over the wire: %q", got)
	}
	if !masked.GetCloudflare().GetApiTokenSet() {
		t.Error("Mask must set api_token_set when a secret is stored")
	}
	if got := masked.GetSmtp().GetHost(); got != "smtp.example.com" {
		t.Errorf("Mask dropped a non-secret field: host=%q", got)
	}
}

func TestEmailCodecMaskClearsFlagWhenNoSecret(t *testing.T) {
	masked := systemcodec.EmailCodec.Mask(systemcodec.EmailProfile{Provider: "smtp"})
	if masked.GetSmtp().GetPasswordSet() {
		t.Error("password_set must be false when no secret is stored")
	}
}

func TestEmailCodecMergeKeepsStoredSecretOnEmptyUpdate(t *testing.T) {
	stored := systemcodec.EmailProfile{
		Smtp:       systemcodec.SmtpConfig{Password: "stored-pw"},
		Cloudflare: systemcodec.CloudflareEmailConfig{ApiToken: "stored-tok"},
	}
	updated := systemcodec.EmailProfile{
		Smtp:       systemcodec.SmtpConfig{Password: ""},
		Cloudflare: systemcodec.CloudflareEmailConfig{ApiToken: ""},
	}

	merged := systemcodec.EmailCodec.Merge(updated, stored)

	if merged.Smtp.Password != "stored-pw" {
		t.Errorf("Merge wiped the stored smtp password: %q", merged.Smtp.Password)
	}
	if merged.Cloudflare.ApiToken != "stored-tok" {
		t.Errorf("Merge wiped the stored cloudflare token: %q", merged.Cloudflare.ApiToken)
	}
}

func TestEmailCodecMergeTakesNewSecretWhenProvided(t *testing.T) {
	stored := systemcodec.EmailProfile{Smtp: systemcodec.SmtpConfig{Password: "old"}}
	updated := systemcodec.EmailProfile{Smtp: systemcodec.SmtpConfig{Password: "new"}}

	if merged := systemcodec.EmailCodec.Merge(updated, stored); merged.Smtp.Password != "new" {
		t.Errorf("Merge must take the newly provided secret, got %q", merged.Smtp.Password)
	}
}

func TestEmailCodecFromProtoReadsEveryField(t *testing.T) {
	in := &systemv1.EmailProfile{
		Id:          "1",
		Name:        "prod",
		Provider:    "smtp",
		FromAddress: "noreply@example.com",
		FromName:    "Prod",
		Smtp: &systemv1.SmtpConfig{
			Host: "smtp.example.com", Port: 587, Username: "u", Password: "pw", Encryption: "starttls",
		},
		Cloudflare: &systemv1.CloudflareEmailConfig{AccountId: "acc", ApiToken: "tok"},
	}

	got := systemcodec.EmailCodec.FromProto(in)

	if got.Name != "prod" || got.FromAddress != "noreply@example.com" {
		t.Errorf("FromProto lost a scalar field: %+v", got)
	}
	if got.Smtp.Port != 587 || got.Smtp.Username != "u" || got.Cloudflare.AccountId != "acc" {
		t.Errorf("FromProto lost a nested field: %+v", got)
	}
	if got.Smtp.Password != "pw" || got.Cloudflare.ApiToken != "tok" {
		t.Error("FromProto must read secret values into storage")
	}
}

func TestOauthCodecMergeKeepsImmutableKeyAndSecrets(t *testing.T) {
	stored := systemcodec.OauthProfile{
		Key:    "google",
		Oidc:   systemcodec.OidcOauthConfig{ClientSecret: "stored-cs"},
		Wechat: systemcodec.WechatOauthConfig{AppSecret: "stored-as"},
	}
	updated := systemcodec.OauthProfile{
		Key:    "changed",
		Oidc:   systemcodec.OidcOauthConfig{ClientSecret: ""},
		Wechat: systemcodec.WechatOauthConfig{AppSecret: ""},
	}

	merged := systemcodec.OauthCodec.Merge(updated, stored)

	if merged.Key != "google" {
		t.Errorf("Merge must keep the immutable stored key, got %q", merged.Key)
	}
	if merged.Oidc.ClientSecret != "stored-cs" || merged.Wechat.AppSecret != "stored-as" {
		t.Error("Merge must keep stored oauth secrets on an empty update")
	}
}

func TestPaymentCodecRoundTripsMethodsAndKeepsSecrets(t *testing.T) {
	in := &systemv1.PaymentProfile{
		Id:       "1",
		Name:     "prod",
		Provider: "wechat",
		Methods:  []string{"native", "jsapi"},
		Wechat:   &systemv1.WechatPaymentConfig{MchId: "m", MchPrivateKey: "pk", ApiV3Key: "v3"},
	}

	got := systemcodec.PaymentCodec.FromProto(in)
	if len(got.Methods) != 2 || got.Methods[0] != "native" {
		t.Errorf("FromProto lost the repeated methods: %v", got.Methods)
	}

	masked := systemcodec.PaymentCodec.Mask(got)
	if masked.GetWechat().GetMchPrivateKey() != "" || masked.GetWechat().GetApiV3Key() != "" {
		t.Error("Mask leaked a payment secret")
	}
	if !masked.GetWechat().GetMchPrivateKeySet() || !masked.GetWechat().GetApiV3KeySet() {
		t.Error("Mask must set both payment secret flags")
	}
	if len(masked.GetMethods()) != 2 || masked.GetMethods()[1] != "jsapi" {
		t.Error("Mask lost the repeated methods")
	}

	stored := systemcodec.PaymentProfile{
		Alipay: systemcodec.AlipayPaymentConfig{AppPrivateKey: "a"},
		Wechat: systemcodec.WechatPaymentConfig{MchPrivateKey: "m", ApiV3Key: "v"},
	}
	merged := systemcodec.PaymentCodec.Merge(systemcodec.PaymentProfile{}, stored)
	if merged.Alipay.AppPrivateKey != "a" || merged.Wechat.MchPrivateKey != "m" || merged.Wechat.ApiV3Key != "v" {
		t.Error("Merge must keep every stored payment secret on an empty update")
	}
}
