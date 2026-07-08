// Package mail sends email through connection profiles configured in system
// settings. Each provider is a driver behind the Sender seam with its own
// config shape: SMTP (wneessen/go-mail; net/smtp is frozen upstream) and the
// Cloudflare Email Service REST API. Profiles are bound to code-defined
// purposes; clients are built per send so config changes apply without a
// restart.
package mail

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	gomail "github.com/wneessen/go-mail"

	"github.com/imbytecat/moonbase/server/internal/integration"
	"github.com/imbytecat/moonbase/server/internal/settings"
	"github.com/imbytecat/moonbase/server/internal/systemcodec"
)

// Email purposes are code, not data: each is a fixed slot the application
// sends through, and operators bind each one to a connection profile. Adding
// a feature that sends email = adding a purpose here.
const (
	// PurposeAuth carries authentication mail: verification codes and links,
	// password resets.
	PurposeAuth = "auth"
)

// Purposes is the catalog served to the admin UI, in display order.
var Purposes = integration.Catalog{PurposeAuth}

var ErrNotConfigured = fmt.Errorf("email is not configured")

// Sender is the semantic seam business code depends on: recipient, subject
// and body in, addressed by purpose.
type Sender interface {
	Send(ctx context.Context, purpose, to, subject, textBody string) error
	SendWith(ctx context.Context, profile systemcodec.EmailProfile, to, subject, textBody string) error
}

type sendFunc = func(c *Client, ctx context.Context, p systemcodec.EmailProfile, to, subject, textBody string) error

var drivers = integration.Registry[systemcodec.EmailProfile, sendFunc]{
	"smtp": {
		Usable: func(p systemcodec.EmailProfile) bool { return p.Smtp.Host != "" },
		Ops:    (*Client).sendSmtp,
	},
	"cloudflare": {
		Usable: func(p systemcodec.EmailProfile) bool {
			return p.Cloudflare.AccountId != "" && p.Cloudflare.ApiToken != ""
		},
		Ops: (*Client).sendCloudflare,
	},
}

// Providers lists registered driver names, sorted.
func Providers() []string {
	return drivers.Names()
}

// ProfileUsable reports whether the profile's driver is fully configured —
// the same gate SendWith enforces.
func ProfileUsable(p systemcodec.EmailProfile) bool {
	return p.FromAddress != "" && drivers.ProfileUsable(p)
}

// Usable reports whether the purpose resolves to a usable profile — shared
// with GetAuthConfig capability flags.
func Usable(cfg settings.Email, purpose string) bool {
	p, ok := cfg.ProfileFor(purpose)
	return ok && ProfileUsable(p)
}

type Client struct {
	store *settings.Store
	http  *http.Client
}

func NewClient(store *settings.Store) *Client {
	return &Client{store: store, http: &http.Client{Timeout: 30 * time.Second}}
}

var _ Sender = (*Client)(nil)

func (c *Client) Send(ctx context.Context, purpose, to, subject, textBody string) error {
	cfg, err := c.store.Email(ctx)
	if err != nil {
		return err
	}
	p, ok := cfg.ProfileFor(purpose)
	if !ok {
		return ErrNotConfigured
	}
	return c.SendWith(ctx, p, to, subject, textBody)
}

func (c *Client) SendWith(ctx context.Context, profile systemcodec.EmailProfile, to, subject, textBody string) error {
	if !ProfileUsable(profile) {
		return ErrNotConfigured
	}
	return drivers[profile.Provider].Ops(c, ctx, profile, to, subject, textBody)
}

func (c *Client) sendSmtp(ctx context.Context, p systemcodec.EmailProfile, to, subject, textBody string) error {
	smtp := p.Smtp

	msg := gomail.NewMsg()
	if p.FromName != "" {
		if err := msg.FromFormat(p.FromName, p.FromAddress); err != nil {
			return fmt.Errorf("invalid from address: %w", err)
		}
	} else if err := msg.From(p.FromAddress); err != nil {
		return fmt.Errorf("invalid from address: %w", err)
	}
	if err := msg.To(to); err != nil {
		return fmt.Errorf("invalid recipient: %w", err)
	}
	msg.Subject(subject)
	msg.SetBodyString(gomail.TypeTextPlain, textBody)

	opts := []gomail.Option{gomail.WithPort(int(smtp.Port))}
	switch smtp.Encryption {
	case "ssl":
		opts = append(opts, gomail.WithSSLPort(false))
	case "none":
		opts = append(opts, gomail.WithTLSPolicy(gomail.NoTLS))
	default: // starttls
		opts = append(opts, gomail.WithTLSPolicy(gomail.TLSMandatory))
	}
	if smtp.Username != "" {
		opts = append(opts,
			gomail.WithSMTPAuth(gomail.SMTPAuthAutoDiscover),
			gomail.WithUsername(smtp.Username),
			gomail.WithPassword(smtp.Password),
		)
	}

	client, err := gomail.NewClient(smtp.Host, opts...)
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
func (c *Client) sendCloudflare(ctx context.Context, p systemcodec.EmailProfile, to, subject, textBody string) error {
	cf := p.Cloudflare

	from := p.FromAddress
	if p.FromName != "" {
		from = fmt.Sprintf("%s <%s>", p.FromName, p.FromAddress)
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
		"https://api.cloudflare.com/client/v4/accounts/%s/email/sending/send", cf.AccountId)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+cf.ApiToken)
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
