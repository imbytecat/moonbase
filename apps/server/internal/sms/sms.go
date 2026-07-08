// Package sms delivers verification codes through connection profiles
// configured in system settings. Each provider is a driver behind the Sender
// seam with its own config shape (Aliyun dysmsapi, Tencent Cloud SMS); the
// caller passes semantic inputs (E.164 phone + code) and drivers own
// provider-specific quirks — template parameter shape and phone-number
// formatting. Profiles are bound to code-defined purposes; clients are built
// per send so config changes apply without a restart.
package sms

import (
	"context"
	"fmt"

	openapiutil "github.com/alibabacloud-go/darabonba-openapi/v2/utils"
	dysmsapi "github.com/alibabacloud-go/dysmsapi-20170525/v5/client"
	tccommon "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	tcprofile "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/profile"
	tcsms "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/sms/v20210111"

	"github.com/imbytecat/moonbase/server/internal/integration"
	"github.com/imbytecat/moonbase/server/internal/phone"
	"github.com/imbytecat/moonbase/server/internal/settings"
	"github.com/imbytecat/moonbase/server/internal/systemcodec"
)

// SMS purposes are code, not data: each is a fixed slot the application
// sends through, and operators bind each one to a connection profile. Adding
// a feature that sends SMS = adding a purpose here.
const (
	// PurposeVerification carries verification codes: login, phone binding,
	// phone-verified signup.
	PurposeVerification = "verification"
)

// Purposes is the catalog served to the admin UI, in display order.
var Purposes = integration.Catalog{PurposeVerification}

var ErrNotConfigured = fmt.Errorf("sms is not configured")

// Sender delivers a verification code to an E.164 phone number, addressed by
// purpose. SendTemplateWith delivers arbitrary template content (cloud SMS
// only accepts pre-approved templates, so the caller names one whose single
// variable receives the content).
type Sender interface {
	SendCode(ctx context.Context, purpose, e164, code string) error
	SendCodeWith(ctx context.Context, profile systemcodec.SmsProfile, e164, code string) error
	SendTemplateWith(ctx context.Context, profile systemcodec.SmsProfile, templateCode, e164, content string) error
}

type sendFunc = func(ctx context.Context, p systemcodec.SmsProfile, templateCode, e164, content string) error

var drivers = integration.Registry[systemcodec.SmsProfile, sendFunc]{
	"aliyun": {
		Usable: func(p systemcodec.SmsProfile) bool {
			a := p.Aliyun
			return a.AccessKeyId != "" && a.SignName != "" && a.TemplateCode != ""
		},
		Ops: func(_ context.Context, p systemcodec.SmsProfile, templateCode, e164, content string) error {
			return sendAliyun(p.Aliyun, templateCode, e164, content)
		},
	},
	"tencent": {
		Usable: func(p systemcodec.SmsProfile) bool {
			t := p.Tencent
			return t.SecretId != "" && t.SdkAppId != "" && t.SignName != "" && t.TemplateId != ""
		},
		Ops: func(ctx context.Context, p systemcodec.SmsProfile, templateCode, e164, content string) error {
			return sendTencent(ctx, p.Tencent, templateCode, e164, content)
		},
	},
}

// Providers lists registered driver names, sorted.
func Providers() []string {
	return drivers.Names()
}

// ProfileUsable reports whether the profile's driver is fully configured —
// the same gate SendCodeWith enforces.
func ProfileUsable(p systemcodec.SmsProfile) bool {
	return drivers.ProfileUsable(p)
}

// Usable reports whether the purpose resolves to a usable profile — shared
// with GetAuthConfig capability flags.
func Usable(cfg settings.Sms, purpose string) bool {
	p, ok := cfg.ProfileFor(purpose)
	return ok && ProfileUsable(p)
}

type Client struct {
	store *settings.Store
}

func NewClient(store *settings.Store) *Client {
	return &Client{store: store}
}

var _ Sender = (*Client)(nil)

func (c *Client) SendCode(ctx context.Context, purpose, e164, code string) error {
	cfg, err := c.store.Sms(ctx)
	if err != nil {
		return err
	}
	p, ok := cfg.ProfileFor(purpose)
	if !ok {
		return ErrNotConfigured
	}
	return c.SendCodeWith(ctx, p, e164, code)
}

func (c *Client) SendCodeWith(ctx context.Context, profile systemcodec.SmsProfile, e164, code string) error {
	// An empty templateCode falls back to the profile's verification-code
	// template inside each driver.
	return c.SendTemplateWith(ctx, profile, "", e164, code)
}

func (c *Client) SendTemplateWith(ctx context.Context, profile systemcodec.SmsProfile, templateCode, e164, content string) error {
	send, ok := drivers.OpsFor(profile)
	if !ok {
		return ErrNotConfigured
	}
	return send(ctx, profile, templateCode, e164, content)
}

// sendAliyun formats per dysmsapi expectations: national digits for mainland
// CN numbers, E.164 otherwise; the template takes JSON {"code": "..."}.
func sendAliyun(cfg systemcodec.AliyunSmsConfig, templateCode, e164, content string) error {
	target := e164
	if phone.RegionOf(e164) == "CN" {
		national, err := phone.NationalNumber(e164)
		if err != nil {
			return err
		}
		target = national
	}

	client, err := dysmsapi.NewClient(&openapiutil.Config{
		AccessKeyId:     &cfg.AccessKeyId,
		AccessKeySecret: &cfg.AccessKeySecret,
		Endpoint:        ptr("dysmsapi.aliyuncs.com"),
	})
	if err != nil {
		return fmt.Errorf("create sms client: %w", err)
	}

	if templateCode == "" {
		templateCode = cfg.TemplateCode
	}
	templateParam := fmt.Sprintf(`{"code":%q}`, content)
	resp, err := client.SendSms(&dysmsapi.SendSmsRequest{
		PhoneNumbers:  &target,
		SignName:      &cfg.SignName,
		TemplateCode:  &templateCode,
		TemplateParam: &templateParam,
	})
	if err != nil {
		return fmt.Errorf("send sms: %w", err)
	}
	if body := resp.Body; body != nil && body.Code != nil && *body.Code != "OK" {
		msg := ""
		if body.Message != nil {
			msg = *body.Message
		}
		return fmt.Errorf("send sms: %s (%s)", msg, *body.Code)
	}
	return nil
}

// sendTencent formats per Tencent Cloud SMS expectations: +E.164 phone
// numbers and positional template params (the content is param {1}).
func sendTencent(ctx context.Context, cfg systemcodec.TencentSmsConfig, templateCode, e164, content string) error {
	region := cfg.Region
	if region == "" {
		region = "ap-guangzhou"
	}

	credential := tccommon.NewCredential(cfg.SecretId, cfg.SecretKey)
	client, err := tcsms.NewClient(credential, region, tcprofile.NewClientProfile())
	if err != nil {
		return fmt.Errorf("create sms client: %w", err)
	}

	if templateCode == "" {
		templateCode = cfg.TemplateId
	}
	req := tcsms.NewSendSmsRequest()
	req.SetContext(ctx)
	req.PhoneNumberSet = []*string{&e164}
	req.SmsSdkAppId = &cfg.SdkAppId
	req.SignName = &cfg.SignName
	req.TemplateId = &templateCode
	req.TemplateParamSet = []*string{&content}

	resp, err := client.SendSms(req)
	if err != nil {
		return fmt.Errorf("send sms: %w", err)
	}
	for _, status := range resp.Response.SendStatusSet {
		if status.Code != nil && *status.Code != "Ok" {
			msg := ""
			if status.Message != nil {
				msg = *status.Message
			}
			return fmt.Errorf("send sms: %s (%s)", msg, *status.Code)
		}
	}
	return nil
}

func ptr[T any](v T) *T { return &v }
