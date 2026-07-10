package llm

import (
	"github.com/imbytecat/moonbase/integrations/core/integration"
	kitsettings "github.com/imbytecat/moonbase/integrations/core/settings"
	llmint "github.com/imbytecat/moonbase/integrations/llm"
)

const PurposeChat = "chat"

var Purposes = integration.Catalog{{
	Key: PurposeChat, Name: "通用对话", Description: "供业务功能调用通用对话模型", Cardinality: integration.Single,
}}

var Registry = llmint.Registry
var ErrNotConfigured = llmint.ErrNotConfigured

type Config = llmint.Config
type Loader = llmint.Loader
type Chatter = llmint.Chatter
type Client = llmint.Client

func NewClient(load Loader) *Client { return llmint.NewClient(load) }
func Providers() []string           { return Registry.Names() }
func ProfileUsable(profile kitsettings.GenericProfile) bool {
	return Registry.ProfileUsable(profile.Provider, profile.Config)
}
func Usable(cfg Config, purpose string) bool {
	profile, ok := cfg.ProfileFor(purpose)
	return ok && ProfileUsable(profile)
}
