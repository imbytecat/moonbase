package turnstile

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	captchaint "github.com/imbytecat/moonbase/integrations/captcha"
	"github.com/imbytecat/moonbase/integrations/core/config"
	"github.com/imbytecat/moonbase/integrations/core/integration"
)

type providerConfig struct {
	SiteKey   string `json:"siteKey" jsonschema:"required,title=站点密钥,minLength=1,maxLength=128"`
	SecretKey string `json:"secretKey" jsonschema:"required,title=服务端密钥,minLength=1,maxLength=128"`
}

func New(client *http.Client) captchaint.Registration {
	return captchaint.Register("turnstile", integration.Presentation{Name: "Cloudflare 人机验证", Description: "通过托管挑战识别自动化访问", Color: "#f6821f", IconRef: "antd:SafetyCertificateOutlined"}, config.MustContract[providerConfig](config.Policy{Secrets: []string{"/secretKey"}}), captchaint.Operations[providerConfig]{SiteKey: func(c providerConfig) string { return c.SiteKey }, Verify: func(ctx context.Context, c providerConfig, token, ip string) error {
		return verify(client, ctx, c, token, ip)
	}})
}
func verify(client *http.Client, ctx context.Context, c providerConfig, token, ip string) error {
	form := url.Values{"secret": {c.SecretKey}, "response": {token}}
	if ip != "" {
		form.Set("remoteip", ip)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://challenges.cloudflare.com/turnstile/v0/siteverify", strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("turnstile verify: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	var out struct {
		Success    bool     `json:"success"`
		ErrorCodes []string `json:"error-codes"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return fmt.Errorf("turnstile decode: %w", err)
	}
	if !out.Success {
		return fmt.Errorf("captcha verification failed: %s", strings.Join(out.ErrorCodes, ","))
	}
	return nil
}
