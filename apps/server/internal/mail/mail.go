package mail

import (
	"github.com/imbytecat/moonbase/integrations/core/integration"
	kitsettings "github.com/imbytecat/moonbase/integrations/core/settings"
	email "github.com/imbytecat/moonbase/integrations/email"
)

const PurposeAuth = "auth"

var Purposes = integration.Catalog{{
	Key: PurposeAuth, Name: "认证邮件", Description: "邮箱验证、密码重置与登录验证码", Cardinality: integration.Single,
}}

var Registry = email.Registry
var ErrNotConfigured = email.ErrNotConfigured

type Config = email.Config
type Loader = email.Loader
type Sender = email.Sender
type Client = email.Client

func NewClient(load Loader) *Client { return email.NewClient(load) }
func Providers() []string           { return Registry.Names() }
func ProfileUsable(profile kitsettings.GenericProfile) bool {
	return Registry.ProfileUsable(profile.Provider, profile.Config)
}
func Usable(cfg Config, purpose string) bool {
	profile, ok := cfg.ProfileFor(purpose)
	return ok && ProfileUsable(profile)
}
