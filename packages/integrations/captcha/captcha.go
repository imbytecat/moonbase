// Package captcha verifies CAPTCHA responses through connection profiles
// configured in system settings. Each provider is a driver with its own
// config shape (Cloudflare Turnstile, Geetest v4). Profiles are bound to
// code-defined purposes; an unbound purpose means pass-through — the login
// flow stays usable on a fresh install with zero configuration.
package captcha

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/imbytecat/moonbase/integrations/core/integration"
	kitsettings "github.com/imbytecat/moonbase/integrations/core/settings"
)

type Config = kitsettings.Integration[kitsettings.GenericProfile]

type Store interface {
	Captcha(ctx context.Context) (Config, error)
	CaptchaAltchaKey(ctx context.Context) ([]byte, error)
}

type Verifier interface {
	// Enabled reports whether the purpose resolves to a fully-configured
	// profile.
	Enabled(ctx context.Context, purpose string) (bool, error)
	// Verify checks the client-side token; nil means pass. Callers treat any
	// error as "reject the request" (fail closed once the purpose is bound).
	Verify(ctx context.Context, purpose, token, remoteIP string) error
}

// captchaOps bundles the per-provider surface: the public site key served to
// widgets and server-side token verification.
type captchaOps struct {
	siteKey func(config map[string]any) string
	verify  func(c *Client, ctx context.Context, config map[string]any, token, remoteIP string) error
}

var Registry = integration.MustRegistry([]integration.Entry[captchaOps]{
	{
		Key:          "turnstile",
		Presentation: integration.Presentation{Name: "Cloudflare 人机验证", Description: "通过托管挑战识别自动化访问", Color: "#f6821f", IconRef: "antd:SafetyCertificateOutlined"},
		Config:       turnstileSchema,
		Ops: captchaOps{
			siteKey: func(config map[string]any) string { return cfgStr(config, "siteKey") },
			verify:  (*Client).verifyTurnstile,
		},
	},
	{
		Key:          "geetest",
		Presentation: integration.Presentation{Name: "极验行为验证", Description: "通过行为挑战识别自动化访问", Color: "#3b82f6", IconRef: "antd:SafetyOutlined"},
		Config:       geetestSchema,
		Ops: captchaOps{
			siteKey: func(config map[string]any) string { return cfgStr(config, "captchaId") },
			verify:  (*Client).verifyGeetest,
		},
	},
	// The built-in ALTCHA driver needs no keys: the widget fetches its
	// challenge from /api/captcha/altcha/challenge, so siteKey is empty on
	// purpose.
	{
		Key:          "altcha",
		Presentation: integration.Presentation{Name: "内置工作量验证", Description: "由本站签发并校验无外部依赖的计算挑战", Color: "#52c41a", IconRef: "antd:ThunderboltOutlined"},
		Config:       altchaSchema,
		Ops: captchaOps{
			siteKey: func(map[string]any) string { return "" },
			verify:  (*Client).verifyAltcha,
		},
	},
})

func Providers() []string { return Registry.Names() }

// ProfileUsable reports whether the profile's driver is fully configured.
func ProfileUsable(p kitsettings.GenericProfile) bool {
	return Registry.ProfileUsable(p.Provider, p.Config)
}

// Widget returns the provider name and public site key the login page needs
// to render the challenge for a purpose; ok=false means pass-through.
func Widget(cfg Config, purpose string) (provider, siteKey string, ok bool) {
	p, found := cfg.ProfileFor(purpose)
	if !found {
		return "", "", false
	}
	ops, ok := Registry.OpsFor(p.Provider, p.Config)
	if !ok {
		return "", "", false
	}
	return p.Provider, ops.siteKey(p.Config), true
}

type Client struct {
	store        Store
	http         *http.Client
	altchaReplay *replayCache
}

func NewClient(store Store) *Client {
	return &Client{
		store:        store,
		http:         &http.Client{Timeout: 10 * time.Second},
		altchaReplay: newReplayCache(),
	}
}

var _ Verifier = (*Client)(nil)

func (c *Client) Enabled(ctx context.Context, purpose string) (bool, error) {
	cfg, err := c.store.Captcha(ctx)
	if err != nil {
		return false, err
	}
	_, _, ok := Widget(cfg, purpose)
	return ok, nil
}

func (c *Client) Verify(ctx context.Context, purpose, token, remoteIP string) error {
	cfg, err := c.store.Captcha(ctx)
	if err != nil {
		return err
	}
	p, found := cfg.ProfileFor(purpose)
	if !found {
		return nil
	}
	ops, ok := Registry.OpsFor(p.Provider, p.Config)
	if !ok {
		return nil
	}
	if token == "" {
		return fmt.Errorf("captcha token required")
	}
	return ops.verify(c, ctx, p.Config, token, remoteIP)
}

// https://developers.cloudflare.com/turnstile/get-started/server-side-validation/
func (c *Client) verifyTurnstile(ctx context.Context, config map[string]any, token, remoteIP string) error {
	form := url.Values{"secret": {cfgStr(config, "secretKey")}, "response": {token}}
	if remoteIP != "" {
		form.Set("remoteip", remoteIP)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://challenges.cloudflare.com/turnstile/v0/siteverify",
		strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.http.Do(req)
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

// Geetest v4 "web integration" server-side validation: the client-side token
// is the JSON blob the widget produces (lot_number, captcha_output, pass_token,
// gen_time), POSTed to the validate endpoint signed with HMAC-SHA256.
// https://docs.geetest.com/BehaviorVerification/apirefer/api/web
func (c *Client) verifyGeetest(ctx context.Context, config map[string]any, token, _ string) error {
	var payload struct {
		LotNumber     string `json:"lot_number"`
		CaptchaOutput string `json:"captcha_output"`
		PassToken     string `json:"pass_token"`
		GenTime       string `json:"gen_time"`
	}
	if err := json.Unmarshal([]byte(token), &payload); err != nil {
		return fmt.Errorf("geetest token decode: %w", err)
	}

	form := url.Values{
		"lot_number":     {payload.LotNumber},
		"captcha_output": {payload.CaptchaOutput},
		"pass_token":     {payload.PassToken},
		"gen_time":       {payload.GenTime},
		"sign_token":     {geetestSign(cfgStr(config, "captchaKey"), payload.LotNumber)},
	}
	endpoint := "https://gcaptcha4.geetest.com/validate?captcha_id=" + url.QueryEscape(cfgStr(config, "captchaId"))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint,
		strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.http.Do(req)
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

func cfgStr(config map[string]any, key string) string {
	s, _ := config[key].(string)
	return s
}
