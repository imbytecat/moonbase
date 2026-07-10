package integrations_test

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"testing"

	"github.com/imbytecat/moonbase/integrations/captcha"
	"github.com/imbytecat/moonbase/integrations/captcha/altcha"
	"github.com/imbytecat/moonbase/integrations/captcha/geetest"
	"github.com/imbytecat/moonbase/integrations/captcha/turnstile"
	"github.com/imbytecat/moonbase/integrations/email"
	"github.com/imbytecat/moonbase/integrations/email/cloudflare"
	"github.com/imbytecat/moonbase/integrations/email/smtp"
	"github.com/imbytecat/moonbase/integrations/llm"
	"github.com/imbytecat/moonbase/integrations/llm/anthropic"
	"github.com/imbytecat/moonbase/integrations/llm/openai"
	"github.com/imbytecat/moonbase/integrations/oauth"
	"github.com/imbytecat/moonbase/integrations/oauth/oidc"
	oauthwechat "github.com/imbytecat/moonbase/integrations/oauth/wechat"
	pay "github.com/imbytecat/moonbase/integrations/payment"
	"github.com/imbytecat/moonbase/integrations/payment/alipay"
	paymentwechat "github.com/imbytecat/moonbase/integrations/payment/wechat"
	"github.com/imbytecat/moonbase/integrations/sms"
	"github.com/imbytecat/moonbase/integrations/sms/aliyun"
	"github.com/imbytecat/moonbase/integrations/sms/tencent"
	"github.com/imbytecat/moonbase/integrations/storage"
	"github.com/imbytecat/moonbase/integrations/storage/local"
	"github.com/imbytecat/moonbase/integrations/storage/s3"
)

func TestExportAllProviderSchemasForWeb(t *testing.T) {
	output := os.Getenv("MOONBASE_PROVIDER_SCHEMA_OUTPUT")
	if output == "" {
		t.Skip("only used by the Web cross-runtime contract test")
	}
	keyLoader := func(context.Context) ([]byte, error) { return []byte("schema-test-key"), nil }
	all := map[string]any{
		"email":   email.MustRegistry(smtp.New(), cloudflare.New(nil)).Descriptors(),
		"payment": pay.MustRegistry(alipay.New(), paymentwechat.New()).Descriptors(),
		"oauth":   oauth.MustRegistry(oidc.New(), oauthwechat.New()).Descriptors(),
		"sms":     sms.MustRegistry(aliyun.New(), tencent.New()).Descriptors(),
		"storage": storage.MustRegistry(s3.New(), local.New()).Descriptors(),
		"captcha": captcha.MustRegistry(turnstile.New(http.DefaultClient), geetest.New(http.DefaultClient), altcha.New(keyLoader)).Descriptors(),
		"llm":     llm.MustRegistry(openai.New(), anthropic.New()).Descriptors(),
	}
	raw, err := json.Marshal(all)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(output, raw, 0o600); err != nil {
		t.Fatal(err)
	}
}
