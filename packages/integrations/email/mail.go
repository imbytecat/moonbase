// Package email sends email through connection profiles configured in system
// settings. Each provider is a driver behind the Sender seam with its own
// config shape: SMTP (wneessen/go-mail; net/smtp is frozen upstream) and the
// Cloudflare Email Service REST API. Profiles are bound to code-defined
// purposes; clients are built per send so config changes apply without a
// restart.
package email

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	gomail "github.com/wneessen/go-mail"

	"github.com/imbytecat/moonbase/integrations/core/integration"
	kitsettings "github.com/imbytecat/moonbase/integrations/core/settings"
)

var ErrNotConfigured = fmt.Errorf("email is not configured")

type Config = kitsettings.Integration[kitsettings.GenericProfile]

type Loader func(ctx context.Context) (Config, error)

// Sender is the semantic seam business code depends on: recipient, subject
// and body in, addressed by purpose.
type Sender interface {
	Send(ctx context.Context, purpose, to, subject, textBody string) error
	SendWith(ctx context.Context, profile kitsettings.GenericProfile, to, subject, textBody string) error
}

type sendFunc = func(c *Client, ctx context.Context, config map[string]any, to, subject, textBody string) error

var Registry = integration.MustRegistry([]integration.Entry[sendFunc]{
	{
		Key:          "smtp",
		Presentation: integration.Presentation{Name: "SMTP 邮件", Description: "通过标准 SMTP 服务器发送邮件", Color: "#1677ff", IconRef: "antd:MailOutlined"},
		Config:       smtpSchema,
		Ops:          (*Client).sendSmtp,
	},
	{
		Key:          "cloudflare",
		Presentation: integration.Presentation{Name: "Cloudflare 邮件", Description: "通过托管邮件发送接口投递邮件", Color: "#f6821f", IconRef: "antd:CloudOutlined"},
		Config:       cloudflareSchema,
		Ops:          (*Client).sendCloudflare,
	},
})

func Providers() []string { return Registry.Names() }

// ProfileUsable reports whether the profile's driver is fully configured —
// the same gate SendWith enforces.
func ProfileUsable(p kitsettings.GenericProfile) bool {
	return Registry.ProfileUsable(p.Provider, p.Config)
}

// Usable reports whether the purpose resolves to a usable profile — shared
// with GetAuthConfig capability flags.
func Usable(cfg Config, purpose string) bool {
	p, ok := cfg.ProfileFor(purpose)
	return ok && ProfileUsable(p)
}

type Client struct {
	load Loader
	http *http.Client
}

func NewClient(load Loader) *Client {
	return &Client{load: load, http: &http.Client{Timeout: 30 * time.Second}}
}

var _ Sender = (*Client)(nil)

func (c *Client) Send(ctx context.Context, purpose, to, subject, textBody string) error {
	cfg, err := c.load(ctx)
	if err != nil {
		return err
	}
	p, ok := cfg.ProfileFor(purpose)
	if !ok {
		return ErrNotConfigured
	}
	return c.SendWith(ctx, p, to, subject, textBody)
}

func (c *Client) SendWith(ctx context.Context, profile kitsettings.GenericProfile, to, subject, textBody string) error {
	send, ok := Registry.OpsFor(profile.Provider, profile.Config)
	if !ok {
		return ErrNotConfigured
	}
	return send(c, ctx, profile.Config, to, subject, textBody)
}

func (c *Client) sendSmtp(ctx context.Context, config map[string]any, to, subject, textBody string) error {
	msg := gomail.NewMsg()
	fromAddress := cfgStr(config, "fromAddress")
	if fromName := cfgStr(config, "fromName"); fromName != "" {
		if err := msg.FromFormat(fromName, fromAddress); err != nil {
			return fmt.Errorf("invalid from address: %w", err)
		}
	} else if err := msg.From(fromAddress); err != nil {
		return fmt.Errorf("invalid from address: %w", err)
	}
	if err := msg.To(to); err != nil {
		return fmt.Errorf("invalid recipient: %w", err)
	}
	msg.Subject(subject)
	msg.SetBodyString(gomail.TypeTextPlain, textBody)

	opts := []gomail.Option{gomail.WithPort(cfgInt(config, "port"))}
	switch cfgStr(config, "encryption") {
	case "ssl":
		opts = append(opts, gomail.WithSSLPort(false))
	case "none":
		opts = append(opts, gomail.WithTLSPolicy(gomail.NoTLS))
	default: // starttls
		opts = append(opts, gomail.WithTLSPolicy(gomail.TLSMandatory))
	}
	if username := cfgStr(config, "username"); username != "" {
		opts = append(opts,
			gomail.WithSMTPAuth(gomail.SMTPAuthAutoDiscover),
			gomail.WithUsername(username),
			gomail.WithPassword(cfgStr(config, "password")),
		)
	}

	client, err := gomail.NewClient(cfgStr(config, "host"), opts...)
	if err != nil {
		return fmt.Errorf("create smtp client: %w", err)
	}
	if err := client.DialAndSendWithContext(ctx, msg); err != nil {
		return fmt.Errorf("send mail: %w", err)
	}
	return nil
}

// sendCloudflare posts to the Cloudflare Email Service (Email Sending) API:
// POST /accounts/{account_id}/email/sending/send with a Bearer token.
// https://developers.cloudflare.com/email-service/api/send-emails/rest-api/
func (c *Client) sendCloudflare(ctx context.Context, config map[string]any, to, subject, textBody string) error {
	from := cfgStr(config, "fromAddress")
	if fromName := cfgStr(config, "fromName"); fromName != "" {
		from = fmt.Sprintf("%s <%s>", fromName, from)
	}
	payload, err := json.Marshal(map[string]string{
		"to":      to,
		"from":    from,
		"subject": subject,
		"text":    textBody,
	})
	if err != nil {
		return fmt.Errorf("encode cloudflare request: %w", err)
	}

	endpoint := fmt.Sprintf(
		"https://api.cloudflare.com/client/v4/accounts/%s/email/sending/send", cfgStr(config, "accountId"))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+cfgStr(config, "apiToken"))
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("cloudflare send: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var out struct {
		Success bool `json:"success"`
		Errors  []struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"errors"`
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err := json.Unmarshal(body, &out); err != nil {
		return fmt.Errorf("cloudflare send: http %d: %s", resp.StatusCode, string(body))
	}
	if !out.Success {
		if len(out.Errors) > 0 {
			return fmt.Errorf("cloudflare send: %s (code %d)", out.Errors[0].Message, out.Errors[0].Code)
		}
		return fmt.Errorf("cloudflare send: http %d", resp.StatusCode)
	}
	return nil
}

func cfgStr(config map[string]any, key string) string {
	s, _ := config[key].(string)
	return s
}

func cfgInt(config map[string]any, key string) int {
	switch v := config[key].(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	default:
		return 0
	}
}
