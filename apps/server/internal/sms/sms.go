package sms

import (
	"context"

	"github.com/imbytecat/moonbase/integrations/core/integration"
	kitsettings "github.com/imbytecat/moonbase/integrations/core/settings"
	smsint "github.com/imbytecat/moonbase/integrations/sms"
)

const PurposeVerification = "verification"

var Purposes = integration.Catalog{
	{
		Key:         PurposeVerification,
		Name:        "短信验证码",
		Description: "登录、注册与手机绑定验证码",
		Cardinality: integration.Single,
	},
}

var ErrNotConfigured = smsint.ErrNotConfigured

type Config = kitsettings.Integration[kitsettings.GenericProfile]
type Loader func(context.Context) (Config, error)

type Sender interface {
	SendCode(ctx context.Context, purpose, e164, code string) error
}

type ProfileSender interface {
	SendCodeWith(ctx context.Context, profile kitsettings.GenericProfile, e164, code string) error
}

type Client struct {
	load     Loader
	registry smsint.Registry
}

func NewClient(load Loader, registry smsint.Registry) *Client {
	return &Client{load: load, registry: registry}
}

var _ Sender = (*Client)(nil)
var _ ProfileSender = (*Client)(nil)

func (c *Client) SendCode(ctx context.Context, purpose, e164, code string) error {
	cfg, err := c.load(ctx)
	if err != nil {
		return err
	}
	profile, ok := cfg.ProfileFor(purpose)
	if !ok {
		return ErrNotConfigured
	}
	return c.SendCodeWith(ctx, profile, e164, code)
}

func (c *Client) SendCodeWith(
	ctx context.Context,
	profile kitsettings.GenericProfile,
	e164, code string,
) error {
	return c.registry.SendCode(ctx, profile.Provider, profile.Config, e164, code)
}

func (c *Client) Usable(ctx context.Context, purpose string) (bool, error) {
	cfg, err := c.load(ctx)
	if err != nil {
		return false, err
	}
	profile, ok := cfg.ProfileFor(purpose)
	return ok && c.registry.ConfigUsable(profile.Provider, profile.Config), nil
}
