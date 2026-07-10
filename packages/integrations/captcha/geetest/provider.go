package geetest

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
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
	CaptchaID  string `json:"captchaId"  jsonschema:"required,title=验证 ID,minLength=1,maxLength=128"`
	CaptchaKey string `json:"captchaKey" jsonschema:"required,title=验证密钥,minLength=1,maxLength=128"`
}

func New(client *http.Client) captchaint.Registration {
	return captchaint.Register(
		"geetest",
		integration.Presentation{
			Name:        "极验行为验证",
			Description: "通过行为挑战识别自动化访问",
			Color:       "#3b82f6",
			IconRef:     "antd:SafetyOutlined",
		},
		config.MustContract[providerConfig](config.Policy{Secrets: []string{"/captchaKey"}}),
		captchaint.Operations[providerConfig]{
			SiteKey: func(c providerConfig) string { return c.CaptchaID },
			Verify: func(ctx context.Context, c providerConfig, token, _ string) error {
				return verify(client, ctx, c, token)
			},
		},
	)
}
func verify(client *http.Client, ctx context.Context, c providerConfig, token string) error {
	var p struct {
		LotNumber     string `json:"lot_number"`
		CaptchaOutput string `json:"captcha_output"`
		PassToken     string `json:"pass_token"`
		GenTime       string `json:"gen_time"`
	}
	if err := json.Unmarshal([]byte(token), &p); err != nil {
		return fmt.Errorf("geetest token decode: %w", err)
	}
	form := url.Values{
		"lot_number":     {p.LotNumber},
		"captcha_output": {p.CaptchaOutput},
		"pass_token":     {p.PassToken},
		"gen_time":       {p.GenTime},
		"sign_token":     {sign(c.CaptchaKey, p.LotNumber)},
	}
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		"https://gcaptcha4.geetest.com/validate?captcha_id="+url.QueryEscape(c.CaptchaID),
		strings.NewReader(form.Encode()),
	)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("geetest verify: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	var out struct {
		Result string `json:"result"`
		Reason string `json:"reason"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return fmt.Errorf("geetest decode: %w", err)
	}
	if out.Result != "success" {
		return fmt.Errorf("captcha verification failed: %s", out.Reason)
	}
	return nil
}
func sign(key, lot string) string {
	mac := hmac.New(sha256.New, []byte(key))
	_, _ = mac.Write([]byte(lot))
	return hex.EncodeToString(mac.Sum(nil))
}
