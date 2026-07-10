// Package oauth implements third-party login through connection profiles
// configured in system settings. Each provider is a driver with its own
// config shape behind the Flow seam: "oidc" is any standard OpenID Connect
// provider (endpoints via discovery), "wechat" is the Open Platform QR-login
// dialect. Flows are addressed by profile KEY (the operator-chosen slug in
// identity records and /api/oauth/{key}/... URLs), not provider — several
// OIDC profiles can coexist. Clients are built per exchange so config
// changes apply without a restart.
package oauth

import (
	"context"
	"fmt"
	"net/url"

	"github.com/imbytecat/moonbase/integrations/core/integration"
	kitsettings "github.com/imbytecat/moonbase/integrations/core/settings"
)

var ErrNotConfigured = fmt.Errorf("oauth provider is not configured")

type Config = kitsettings.Integration[kitsettings.GenericProfile]

type Loader func(ctx context.Context) (Config, error)

// ExternalIdentity is the provider-agnostic result of a code exchange: a
// stable subject plus display info for the identity row. ProviderKey is the
// profile key the exchange ran through.
type ExternalIdentity struct {
	ProviderKey string
	ProviderID  string
	Name        string
	AvatarURL   string
}

// FlowSecrets are per-flow values a driver mints at AuthorizeURL time that the
// caller must persist between the redirect and the callback (in the httpOnly
// state cookie) and hand back to Exchange: the OIDC nonce (bound into the ID
// token) and the PKCE code_verifier. They never round-trip through the
// provider's query string — unlike state, which does and guards CSRF. WeChat
// leaves them empty.
type FlowSecrets struct {
	Nonce    string
	Verifier string
}

// Flow abstracts the provider round-trip for handlers (same seam pattern as
// mail.Sender / sms.Sender). key selects the OAuth profile.
type Flow interface {
	AuthorizeURL(ctx context.Context, key, redirectURI, state string) (url string, secrets FlowSecrets, err error)
	Exchange(ctx context.Context, key, code, redirectURI string, secrets FlowSecrets) (ExternalIdentity, error)
}

type oauthOps struct {
	// authorizeURL builds the provider's authorization page URL; state is the
	// CSRF token round-tripped through the provider. It returns any per-flow
	// secrets (OIDC nonce, PKCE verifier) the caller must store for Exchange.
	authorizeURL func(ctx context.Context, config map[string]any, redirectURI, state string) (string, FlowSecrets, error)
	// exchange trades the callback code for the external identity, using the
	// secrets minted at authorize time to verify the ID token and complete PKCE.
	exchange func(ctx context.Context, config map[string]any, code, redirectURI string, secrets FlowSecrets) (ExternalIdentity, error)
}

var Registry = integration.MustRegistry([]integration.Entry[oauthOps]{
	{
		Key:          "oidc",
		Presentation: integration.Presentation{Name: "OpenID Connect", Description: "连接支持发现、ID Token 校验与 PKCE 的身份提供方", Color: "#1677ff", IconRef: "antd:IdcardOutlined"},
		Config:       oidcSchema,
		Ops: oauthOps{
			authorizeURL: oidcAuthorizeURL,
			exchange:     oidcExchange,
		},
	},
	{
		Key:          "wechat",
		Presentation: integration.Presentation{Name: "微信开放平台", Description: "使用网站应用扫码登录", Color: "#07c160", IconRef: "antd:WechatOutlined"},
		Config:       wechatSchema,
		Ops: oauthOps{
			authorizeURL: wechatAuthorizeURL,
			exchange:     wechatExchange,
		},
	},
})

func Providers() []string { return Registry.Names() }

// ProfileUsable reports whether the profile's driver is fully configured.
func ProfileUsable(p kitsettings.GenericProfile) bool {
	return Registry.ProfileUsable(p.Provider, p.Config)
}

// ProviderOption is one login-page entry.
type ProviderOption struct {
	Key      string
	Name     string
	Provider string
}

// UsableProviders lists login options ready to offer: the profiles bound to
// the login purpose, in binding order, filtered to fully-configured drivers.
func UsableProviders(cfg Config, purpose string) []ProviderOption {
	bound := cfg.ProfilesFor(purpose)
	out := make([]ProviderOption, 0, len(bound))
	for _, p := range bound {
		if ProfileUsable(p) {
			out = append(out, ProviderOption{Key: cfgStr(p.Config, "key"), Name: p.Name, Provider: p.Provider})
		}
	}
	return out
}

type Client struct {
	load    Loader
	purpose string
}

func NewClient(load Loader, purpose string) *Client {
	return &Client{load: load, purpose: purpose}
}

var _ Flow = (*Client)(nil)

// profileFor resolves a flow key to a usable profile. The profile must also
// be bound to the login purpose — unbinding retires the /api/oauth/{key}/...
// endpoints, the same gate the login page uses.
func (c *Client) profileFor(ctx context.Context, key string) (kitsettings.GenericProfile, error) {
	cfg, err := c.load(ctx)
	if err != nil {
		return kitsettings.GenericProfile{}, err
	}
	for _, p := range cfg.ProfilesFor(c.purpose) {
		if cfgStr(p.Config, "key") == key && ProfileUsable(p) {
			return p, nil
		}
	}
	return kitsettings.GenericProfile{}, ErrNotConfigured
}

func (c *Client) AuthorizeURL(ctx context.Context, key, redirectURI, state string) (string, FlowSecrets, error) {
	p, err := c.profileFor(ctx, key)
	if err != nil {
		return "", FlowSecrets{}, err
	}
	ops, _ := Registry.OpsFor(p.Provider, p.Config)
	return ops.authorizeURL(ctx, p.Config, redirectURI, state)
}

func (c *Client) Exchange(ctx context.Context, key, code, redirectURI string, secrets FlowSecrets) (ExternalIdentity, error) {
	p, err := c.profileFor(ctx, key)
	if err != nil {
		return ExternalIdentity{}, err
	}
	ops, _ := Registry.OpsFor(p.Provider, p.Config)
	external, err := ops.exchange(ctx, p.Config, code, redirectURI, secrets)
	if err != nil {
		return ExternalIdentity{}, err
	}
	external.ProviderKey = cfgStr(p.Config, "key")
	return external, nil
}

func cfgStr(config map[string]any, key string) string {
	s, _ := config[key].(string)
	return s
}

func encodeQuery(pairs ...string) string {
	q := url.Values{}
	for i := 0; i+1 < len(pairs); i += 2 {
		q.Set(pairs[i], pairs[i+1])
	}
	return q.Encode()
}
