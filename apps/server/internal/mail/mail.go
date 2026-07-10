package mail

import (
	"context"

	"github.com/imbytecat/moonbase/integrations/core/integration"
	kitsettings "github.com/imbytecat/moonbase/integrations/core/settings"
	"github.com/imbytecat/moonbase/integrations/email"
)

const PurposeAuth = "auth"

var Purposes = integration.Catalog{{
	Key: PurposeAuth, Name: "认证邮件", Description: "邮箱验证、密码重置与登录验证码", Cardinality: integration.Single,
}}

var ErrNotConfigured = email.ErrNotConfigured

type Config = kitsettings.Integration[kitsettings.GenericProfile]
type Loader func(context.Context) (Config, error)

// Sender is the application seam used by business code. Purpose resolution
// stays here and never crosses into the reusable email integration.
type Sender interface {
	Send(ctx context.Context, purpose, to, subject, textBody string) error
	Usable(ctx context.Context, purpose string) (bool, error)
}

type ProfileSender interface {
	SendWith(
		ctx context.Context,
		profile kitsettings.GenericProfile,
		to, subject, textBody string,
	) error
}

type Client struct {
	load     Loader
	registry email.Registry
}

func NewClient(load Loader, registry email.Registry) *Client {
	return &Client{load: load, registry: registry}
}

var (
	_ Sender        = (*Client)(nil)
	_ ProfileSender = (*Client)(nil)
)

func (c *Client) Send(ctx context.Context, purpose, to, subject, textBody string) error {
	settings, err := c.load(ctx)
	if err != nil {
		return err
	}
	profile, ok := settings.ProfileFor(purpose)
	if !ok {
		return ErrNotConfigured
	}
	return c.SendWith(ctx, profile, to, subject, textBody)
}

func (c *Client) SendWith(
	ctx context.Context,
	profile kitsettings.GenericProfile,
	to, subject, textBody string,
) error {
	return c.registry.Send(
		ctx,
		profile.Provider,
		profile.Config,
		email.Message{To: to, Subject: subject, TextBody: textBody},
	)
}

func (c *Client) Usable(ctx context.Context, purpose string) (bool, error) {
	settings, err := c.load(ctx)
	if err != nil {
		return false, err
	}
	profile, ok := settings.ProfileFor(purpose)
	return ok && c.registry.ConfigUsable(profile.Provider, profile.Config), nil
}
