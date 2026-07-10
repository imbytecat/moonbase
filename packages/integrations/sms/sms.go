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

	"github.com/imbytecat/moonbase/integrations/core/integration"
	"github.com/imbytecat/moonbase/integrations/core/phone"
	kitsettings "github.com/imbytecat/moonbase/integrations/core/settings"
)

var ErrNotConfigured = fmt.Errorf("sms is not configured")

type Config = kitsettings.Integration[kitsettings.GenericProfile]

type Loader func(ctx context.Context) (Config, error)

// Sender delivers a verification code to an E.164 phone number, addressed by
// purpose. Provider + opaque config select the driver; base has already
// masked, merged and validated the config against the driver's config.
// SendTemplateWith delivers arbitrary template content (cloud SMS only accepts
// pre-approved templates, so the caller names one whose single variable
// receives the content).
type Sender interface {
	SendCode(ctx context.Context, purpose, e164, code string) error
	SendCodeWith(ctx context.Context, provider string, config map[string]any, e164, code string) error
	SendTemplateWith(ctx context.Context, provider string, config map[string]any, templateCode, e164, content string) error
}

type sendFunc = func(ctx context.Context, config map[string]any, templateCode, e164, content string) error

var Registry = integration.MustRegistry([]integration.Entry[sendFunc]{
	{Key: "aliyun", Presentation: integration.Presentation{Name: "阿里云短信", Description: "通过云短信服务发送验证码与通知", Color: "#ff6a00", IconRef: "antd:AliyunOutlined"}, Config: aliyunSchema, Ops: sendAliyun},
	{Key: "tencent", Presentation: integration.Presentation{Name: "腾讯云短信", Description: "通过云短信服务发送验证码与通知", Color: "#0052d9", IconRef: "antd:MessageOutlined"}, Config: tencentSchema, Ops: sendTencent},
})

func Providers() []string { return Registry.Names() }

// Usable reports whether the purpose resolves to a profile whose driver is
// registered and fully configured — shared with GetAuthConfig capability flags.
func Usable(cfg Config, purpose string) bool {
	p, ok := cfg.ProfileFor(purpose)
	if !ok {
		return false
	}
	return Registry.ProfileUsable(p.Provider, p.Config)
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
	send, ok := Registry.OpsFor(provider, config)
	if !ok {
		return ErrNotConfigured
	}
	return send(ctx, config, templateCode, e164, content)
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
