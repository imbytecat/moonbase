package oauth

import (
	"github.com/imbytecat/moonbase/integrations/core/integration"
	kitsettings "github.com/imbytecat/moonbase/integrations/core/settings"
	oauthint "github.com/imbytecat/moonbase/integrations/oauth"
)

const PurposeLogin = "login"

var Purposes = integration.Catalog{{
	Key: PurposeLogin, Name: "第三方登录", Description: "登录页可用的外部身份提供方", Cardinality: integration.Multiple,
}}

var Registry = oauthint.Registry
var ErrNotConfigured = oauthint.ErrNotConfigured

type Config = oauthint.Config
type Loader = oauthint.Loader
type ExternalIdentity = oauthint.ExternalIdentity
type FlowSecrets = oauthint.FlowSecrets
type Flow = oauthint.Flow
type ProviderOption = oauthint.ProviderOption
type Client = oauthint.Client

func NewClient(load Loader) *Client { return oauthint.NewClient(load, PurposeLogin) }
func Providers() []string           { return Registry.Names() }
func ProfileUsable(profile kitsettings.GenericProfile) bool {
	return Registry.ProfileUsable(profile.Provider, profile.Config)
}
func UsableProviders(cfg Config) []ProviderOption { return oauthint.UsableProviders(cfg, PurposeLogin) }
