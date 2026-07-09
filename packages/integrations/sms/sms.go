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
	"maps"
	"slices"

	openapiutil "github.com/alibabacloud-go/darabonba-openapi/v2/utils"
	dysmsapi "github.com/alibabacloud-go/dysmsapi-20170525/v5/client"
	tccommon "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	tcprofile "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/profile"
	tcsms "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/sms/v20210111"

	"github.com/imbytecat/moonbase/packages/integrations/core/integration"
	"github.com/imbytecat/moonbase/packages/integrations/core/phone"
	"github.com/imbytecat/moonbase/packages/integrations/core/schema"
	kitsettings "github.com/imbytecat/moonbase/packages/integrations/core/settings"
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

type Config = kitsettings.Integration[kitsettings.GenericProfile]

type Loader func(ctx context.Context) (Config, error)

// Sender delivers a verification code to an E.164 phone number, addressed by
// purpose. Provider + opaque config select the driver; base has already
// masked, merged and validated the config against the driver's schema.
// SendTemplateWith delivers arbitrary template content (cloud SMS only accepts
// pre-approved templates, so the caller names one whose single variable
// receives the content).
type Sender interface {
	SendCode(ctx context.Context, purpose, e164, code string) error
	SendCodeWith(ctx context.Context, provider string, config map[string]any, e164, code string) error
	SendTemplateWith(ctx context.Context, provider string, config map[string]any, templateCode, e164, content string) error
}

type sendFunc = func(ctx context.Context, config map[string]any, templateCode, e164, content string) error

type driver struct {
	schema schema.Schema
	send   sendFunc
}

var drivers = map[string]driver{
	"aliyun":  {schema: aliyunSchema, send: sendAliyun},
	"tencent": {schema: tencentSchema, send: sendTencent},
}

// Schemas advertises each provider's config schema, derived from the driver
// registry — the one source base and the admin UI read.
func Schemas() map[string]schema.Schema {
	out := make(map[string]schema.Schema, len(drivers))
	for name, d := range drivers {
		out[name] = d.schema
	}
	return out
}

// Providers lists registered driver names, sorted.
func Providers() []string {
	return slices.Sorted(maps.Keys(drivers))
}

// Usable reports whether the purpose resolves to a profile whose driver is
// registered and fully configured — shared with GetAuthConfig capability flags.
func Usable(cfg Config, purpose string) bool {
	p, ok := cfg.ProfileFor(purpose)
	if !ok {
		return false
	}
	d, ok := drivers[p.Provider]
	return ok && d.schema.Usable(p.Config)
}

type Client struct {
	load Loader
}

func NewClient(load Loader) *Client {
	return &Client{load: load}
}

var _ Sender = (*Client)(nil)

func (c *Client) SendCode(ctx context.Context, purpose, e164, code string) error {
	cfg, err := c.load(ctx)
	if err != nil {
		return err
	}
	p, ok := cfg.ProfileFor(purpose)
	if !ok {
		return ErrNotConfigured
	}
	return c.SendCodeWith(ctx, p.Provider, p.Config, e164, code)
}

func (c *Client) SendCodeWith(ctx context.Context, provider string, config map[string]any, e164, code string) error {
	return c.SendTemplateWith(ctx, provider, config, "", e164, code)
}

func (c *Client) SendTemplateWith(ctx context.Context, provider string, config map[string]any, templateCode, e164, content string) error {
	d, ok := drivers[provider]
	if !ok || !d.schema.Usable(config) {
		return ErrNotConfigured
	}
	return d.send(ctx, config, templateCode, e164, content)
}

func cfgStr(config map[string]any, key string) string {
	s, _ := config[key].(string)
	return s
}

// sendAliyun formats per dysmsapi expectations: national digits for mainland
// CN numbers, E.164 otherwise; the template takes JSON {"code": "..."}.
func sendAliyun(_ context.Context, config map[string]any, templateCode, e164, content string) error {
	target := e164
	if phone.RegionOf(e164) == "CN" {
		national, err := phone.NationalNumber(e164)
		if err != nil {
			return err
		}
		target = national
	}

	akid := cfgStr(config, "accessKeyId")
	aksec := cfgStr(config, "accessKeySecret")
	endpoint := "dysmsapi.aliyuncs.com"
	client, err := dysmsapi.NewClient(&openapiutil.Config{
		AccessKeyId:     &akid,
		AccessKeySecret: &aksec,
		Endpoint:        &endpoint,
	})
	if err != nil {
		return fmt.Errorf("create sms client: %w", err)
	}

	if templateCode == "" {
		templateCode = cfgStr(config, "templateCode")
	}
	signName := cfgStr(config, "signName")
	templateParam := fmt.Sprintf(`{"code":%q}`, content)
	resp, err := client.SendSms(&dysmsapi.SendSmsRequest{
		PhoneNumbers:  &target,
		SignName:      &signName,
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
func sendTencent(ctx context.Context, config map[string]any, templateCode, e164, content string) error {
	region := cfgStr(config, "region")
	if region == "" {
		region = "ap-guangzhou"
	}

	credential := tccommon.NewCredential(cfgStr(config, "secretId"), cfgStr(config, "secretKey"))
	client, err := tcsms.NewClient(credential, region, tcprofile.NewClientProfile())
	if err != nil {
		return fmt.Errorf("create sms client: %w", err)
	}

	if templateCode == "" {
		templateCode = cfgStr(config, "templateId")
	}
	sdkAppId := cfgStr(config, "sdkAppId")
	signName := cfgStr(config, "signName")
	req := tcsms.NewSendSmsRequest()
	req.SetContext(ctx)
	req.PhoneNumberSet = []*string{&e164}
	req.SmsSdkAppId = &sdkAppId
	req.SignName = &signName
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
