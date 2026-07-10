package llm

import (
	"context"

	"github.com/imbytecat/moonbase/integrations/core/integration"
	kitsettings "github.com/imbytecat/moonbase/integrations/core/settings"
	llmint "github.com/imbytecat/moonbase/integrations/llm"
)

const PurposeChat = "chat"

var Purposes = integration.Catalog{
	{Key: PurposeChat, Name: "通用对话", Description: "供业务功能调用通用对话模型", Cardinality: integration.Single},
}
var ErrNotConfigured = llmint.ErrNotConfigured

type Config = kitsettings.Integration[kitsettings.GenericProfile]
type Loader func(context.Context) (Config, error)
type Chatter interface {
	Complete(context.Context, string, string, string) (string, error)
	CompleteWith(context.Context, kitsettings.GenericProfile, string, string) (string, error)
}
type Client struct {
	load     Loader
	registry llmint.Registry
}

func NewClient(load Loader, registry llmint.Registry) *Client {
	return &Client{load: load, registry: registry}
}

var _ Chatter = (*Client)(nil)

func (c *Client) Complete(ctx context.Context, purpose, system, user string) (string, error) {
	cfg, err := c.load(ctx)
	if err != nil {
		return "", err
	}
	p, ok := cfg.ProfileFor(purpose)
	if !ok {
		return "", ErrNotConfigured
	}
	return c.CompleteWith(ctx, p, system, user)
}

func (c *Client) CompleteWith(
	ctx context.Context,
	p kitsettings.GenericProfile,
	system, user string,
) (string, error) {
	return c.registry.Complete(ctx, p.Provider, p.Config, system, user)
}
func Usable(cfg Config, purpose string, r llmint.Registry) bool {
	p, ok := cfg.ProfileFor(purpose)
	return ok && r.ConfigUsable(p.Provider, p.Config)
}
