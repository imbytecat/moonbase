package oauth

import (
	"context"

	"github.com/imbytecat/moonbase/integrations/core/integration"
	kitsettings "github.com/imbytecat/moonbase/integrations/core/settings"
	oauthint "github.com/imbytecat/moonbase/integrations/oauth"
)

const PurposeLogin = "login"

var Purposes = integration.Catalog{{Key: PurposeLogin, Name: "第三方登录", Description: "登录页可用的外部身份提供方", Cardinality: integration.Multiple}}

var ErrNotConfigured = oauthint.ErrNotConfigured

type Config = kitsettings.Integration[kitsettings.GenericProfile]
type Loader func(context.Context) (Config, error)
type ExternalIdentity = oauthint.ExternalIdentity
type FlowSecrets = oauthint.FlowSecrets
type ProviderOption struct{ Key, Name, Provider string }

type Flow interface {
	AuthorizeURL(context.Context, string, string, string) (string, FlowSecrets, error)
	Exchange(context.Context, string, string, string, FlowSecrets) (ExternalIdentity, error)
	ProviderOptions(context.Context) ([]ProviderOption, error)
}

type Client struct {
	load     Loader
	registry oauthint.Registry
}

func NewClient(load Loader, registry oauthint.Registry) *Client {
	return &Client{load: load, registry: registry}
}

func (c *Client) profileFor(ctx context.Context, key string) (kitsettings.GenericProfile, error) {
	cfg, err := c.load(ctx)
	if err != nil {
		return kitsettings.GenericProfile{}, err
	}
	for _, profile := range cfg.ProfilesFor(PurposeLogin) {
		view, valid := c.registry.ViewConfig(profile.Provider, profile.Config)
		if valid && configKey(view.Values) == key {
			return profile, nil
		}
	}
	return kitsettings.GenericProfile{}, ErrNotConfigured
}

func (c *Client) AuthorizeURL(ctx context.Context, key, redirectURI, state string) (string, FlowSecrets, error) {
	profile, err := c.profileFor(ctx, key)
	if err != nil {
		return "", FlowSecrets{}, err
	}
	return c.registry.AuthorizeURL(ctx, profile.Provider, profile.Config, redirectURI, state)
}

func (c *Client) Exchange(ctx context.Context, key, code, redirectURI string, secrets FlowSecrets) (ExternalIdentity, error) {
	profile, err := c.profileFor(ctx, key)
	if err != nil {
		return ExternalIdentity{}, err
	}
	external, err := c.registry.Exchange(ctx, profile.Provider, profile.Config, code, redirectURI, secrets)
	if err != nil {
		return ExternalIdentity{}, err
	}
	external.ProviderKey = key
	return external, nil
}

func (c *Client) ProviderOptions(ctx context.Context) ([]ProviderOption, error) {
	cfg, err := c.load(ctx)
	if err != nil {
		return nil, err
	}
	return UsableProviders(cfg, c.registry), nil
}

func UsableProviders(cfg Config, registry oauthint.Registry) []ProviderOption {
	out := make([]ProviderOption, 0)
	for _, profile := range cfg.ProfilesFor(PurposeLogin) {
		view, valid := registry.ViewConfig(profile.Provider, profile.Config)
		if valid {
			out = append(out, ProviderOption{Key: configKey(view.Values), Name: profile.Name, Provider: profile.Provider})
		}
	}
	return out
}

func configKey(values map[string]any) string { key, _ := values["key"].(string); return key }

var _ Flow = (*Client)(nil)
