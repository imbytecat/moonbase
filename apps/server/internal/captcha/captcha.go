package captcha

import (
	"github.com/imbytecat/moonbase/integrations/captcha"
	"github.com/imbytecat/moonbase/integrations/core/integration"
	kitsettings "github.com/imbytecat/moonbase/integrations/core/settings"
)

const PurposeAuth = "auth"

var Purposes = integration.Catalog{{
	Key: PurposeAuth, Name: "认证人机验证", Description: "保护登录、注册与公开验证码请求", Cardinality: integration.Single,
}}

var Registry = captcha.Registry

type Config = captcha.Config
type Store = captcha.Store
type Verifier = captcha.Verifier
type Client = captcha.Client

func NewClient(store Store) *Client { return captcha.NewClient(store) }
func Providers() []string           { return Registry.Names() }
func ProfileUsable(profile kitsettings.GenericProfile) bool {
	return Registry.ProfileUsable(profile.Provider, profile.Config)
}
func Widget(cfg Config, purpose string) (string, string, bool) { return captcha.Widget(cfg, purpose) }
