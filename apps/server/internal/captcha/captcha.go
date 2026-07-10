package captcha

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	captchaint "github.com/imbytecat/moonbase/integrations/captcha"
	"github.com/imbytecat/moonbase/integrations/core/integration"
	kitsettings "github.com/imbytecat/moonbase/integrations/core/settings"
)

const PurposeAuth = "auth"

var Purposes = integration.Catalog{
	{
		Key:         PurposeAuth,
		Name:        "认证人机验证",
		Description: "保护登录、注册与公开验证码请求",
		Cardinality: integration.Single,
	},
}

type Config = kitsettings.Integration[kitsettings.GenericProfile]
type Store interface {
	Captcha(context.Context) (Config, error)
	CaptchaAltchaKey(context.Context) ([]byte, error)
}
type Verifier interface {
	Enabled(context.Context, string) (bool, error)
	Verify(context.Context, string, string, string) error
	Widget(context.Context, string) (string, string, bool, error)
}
type Client struct {
	store    Store
	registry captchaint.Registry
}

func NewClient(store Store, registry captchaint.Registry) *Client {
	return &Client{store: store, registry: registry}
}

var _ Verifier = (*Client)(nil)

func (c *Client) Enabled(ctx context.Context, purpose string) (bool, error) {
	cfg, err := c.store.Captcha(ctx)
	if err != nil {
		return false, err
	}
	p, ok := cfg.ProfileFor(purpose)
	return ok && c.registry.ConfigUsable(p.Provider, p.Config), nil
}
func (c *Client) Verify(ctx context.Context, purpose, token, ip string) error {
	cfg, err := c.store.Captcha(ctx)
	if err != nil {
		return err
	}
	p, ok := cfg.ProfileFor(purpose)
	if !ok {
		return nil
	}
	if !c.registry.ConfigUsable(p.Provider, p.Config) {
		return captchaint.ErrNotConfigured
	}
	if token == "" {
		return fmt.Errorf("captcha token required")
	}
	return c.registry.Verify(ctx, p.Provider, p.Config, token, ip)
}
func (c *Client) Widget(ctx context.Context, purpose string) (string, string, bool, error) {
	cfg, err := c.store.Captcha(ctx)
	if err != nil {
		return "", "", false, err
	}
	p, ok := cfg.ProfileFor(purpose)
	if !ok {
		return "", "", false, nil
	}
	key, ok := c.registry.Widget(p.Provider, p.Config)
	if !ok {
		return "", "", false, nil
	}
	return p.Provider, key, true, nil
}
func (c *Client) ServeAltchaChallenge(w http.ResponseWriter, r *http.Request) {
	cfg, err := c.store.Captcha(r.Context())
	if err != nil {
		http.Error(w, "internal error", 500)
		return
	}
	p, ok := cfg.ProfileFor(r.URL.Query().Get("purpose"))
	if !ok || p.Provider != "altcha" {
		http.NotFound(w, r)
		return
	}
	challenge, err := c.registry.Challenge(r.Context(), p.Provider, p.Config)
	if err != nil {
		http.Error(w, "internal error", 500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(challenge)
}
