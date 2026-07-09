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
	"maps"
	"net/url"
	"slices"

	"github.com/imbytecat/moonbase/integrations/core/integration"
	"github.com/imbytecat/moonbase/integrations/core/schema"
	kitsettings "github.com/imbytecat/moonbase/integrations/core/settings"
)

var ErrNotConfigured = fmt.Errorf("oauth provider is not configured")

// PurposeLogin is the sign-in page slot; it is multi-valued — every bound
// profile is offered simultaneously.
const PurposeLogin = "login"

// Purposes is the catalog served to the admin UI, in display order.
var Purposes = integration.Catalog{PurposeLogin}

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

type driver struct {
	schema schema.Schema
	ops    oauthOps
}

var drivers = map[string]driver{
	"oidc": {
		schema: oidcSchema,
		ops: oauthOps{
			authorizeURL: oidcAuthorizeURL,
			exchange:     oidcExchange,
		},
	},
	"wechat": {
		schema: wechatSchema,
		ops: oauthOps{
			authorizeURL: wechatAuthorizeURL,
			exchange:     wechatExchange,
		},
	},
}

func Schemas() map[string]schema.Schema {
	out := make(map[string]schema.Schema, len(drivers))
	for name, d := range drivers {
		out[name] = d.schema
	}
	return out
}

// Providers lists registered driver names, sorted.
func Providers() []string {
	return slices.Sorted(maps.Keys(drivers))
}

// ProfileUsable reports whether the profile's driver is fully configured.
func ProfileUsable(p kitsettings.GenericProfile) bool {
	d, ok := drivers[p.Provider]
	return ok && d.schema.Usable(p.Config)
}

// ProviderOption is one login-page entry.
type ProviderOption struct {
	Key      string
	Name     string
	Provider string
}

// UsableProviders lists login options ready to offer: the profiles bound to
// the login purpose, in binding order, filtered to fully-configured drivers.
func UsableProviders(cfg Config) []ProviderOption {
	bound := cfg.ProfilesFor(PurposeLogin)
	out := make([]ProviderOption, 0, len(bound))
	for _, p := range bound {
		if ProfileUsable(p) {
			out = append(out, ProviderOption{Key: cfgStr(p.Config, "key"), Name: p.Name, Provider: p.Provider})
		}
	}
	return out
}

type Client struct {
	load Loader
}

func NewClient(load Loader) *Client {
	return &Client{load: load}
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
	for _, p := range cfg.ProfilesFor(PurposeLogin) {
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
	return drivers[p.Provider].ops.authorizeURL(ctx, p.Config, redirectURI, state)
}

func (c *Client) Exchange(ctx context.Context, key, code, redirectURI string, secrets FlowSecrets) (ExternalIdentity, error) {
	p, err := c.profileFor(ctx, key)
	if err != nil {
		return ExternalIdentity{}, err
	}
	external, err := drivers[p.Provider].ops.exchange(ctx, p.Config, code, redirectURI, secrets)
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
