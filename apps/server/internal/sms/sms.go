package sms

import (
	"github.com/imbytecat/moonbase/integrations/core/integration"
	kitsettings "github.com/imbytecat/moonbase/integrations/core/settings"
	smsint "github.com/imbytecat/moonbase/integrations/sms"
)

const PurposeVerification = "verification"

var Purposes = integration.Catalog{{
	Key: PurposeVerification, Name: "短信验证码", Description: "登录、注册与手机绑定验证码", Cardinality: integration.Single,
}}

var Registry = smsint.Registry
var ErrNotConfigured = smsint.ErrNotConfigured

type Config = smsint.Config
type Loader = smsint.Loader
type Sender = smsint.Sender
type Client = smsint.Client

func NewClient(load Loader) *Client { return smsint.NewClient(load) }
func Providers() []string           { return Registry.Names() }
func ProfileUsable(profile kitsettings.GenericProfile) bool {
	return Registry.ProfileUsable(profile.Provider, profile.Config)
}
func Usable(cfg Config, purpose string) bool {
	profile, ok := cfg.ProfileFor(purpose)
	return ok && ProfileUsable(profile)
}
