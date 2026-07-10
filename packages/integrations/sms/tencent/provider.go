package tencent

import (
	"context"
	"fmt"

	tccommon "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	tcprofile "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/profile"
	tcsms "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/sms/v20210111"

	"github.com/imbytecat/moonbase/integrations/core/config"
	"github.com/imbytecat/moonbase/integrations/core/integration"
	smsint "github.com/imbytecat/moonbase/integrations/sms"
)

type providerConfig struct {
	SecretID   string `json:"secretId"   jsonschema:"required,title=密钥 ID,minLength=1,maxLength=128"`
	SecretKey  string `json:"secretKey"  jsonschema:"required,title=密钥 Key,minLength=1,maxLength=128"`
	SDKAppID   string `json:"sdkAppId"   jsonschema:"required,title=SDK 应用 ID,minLength=1,maxLength=32"`
	SignName   string `json:"signName"   jsonschema:"required,title=短信签名,minLength=1,maxLength=64"`
	TemplateID string `json:"templateId" jsonschema:"required,title=模板 ID,minLength=1,maxLength=32"`
	Region     string `json:"region"     jsonschema:"required,title=区域,description=例如 ap-guangzhou,default=ap-guangzhou,minLength=1,maxLength=32"`
}

func New() smsint.Registration {
	return smsint.Register("tencent", integration.Presentation{
		Name:        "腾讯云短信",
		Description: "通过云短信服务发送验证码与通知",
		Color:       "#0052d9",
		IconRef:     "antd:MessageOutlined",
	}, config.MustContract[providerConfig](config.Policy{Secrets: []string{"/secretKey"}}), send)
}

func send(ctx context.Context, cfg providerConfig, message smsint.Message) error {
	credential := tccommon.NewCredential(cfg.SecretID, cfg.SecretKey)
	client, err := tcsms.NewClient(credential, cfg.Region, tcprofile.NewClientProfile())
	if err != nil {
		return fmt.Errorf("create sms client: %w", err)
	}
	req := newRequest(ctx, cfg, message)
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

func newRequest(
	ctx context.Context,
	cfg providerConfig,
	message smsint.Message,
) *tcsms.SendSmsRequest {
	templateID := message.TemplateCode
	if templateID == "" {
		templateID = cfg.TemplateID
	}
	req := tcsms.NewSendSmsRequest()
	req.SetContext(ctx)
	req.PhoneNumberSet = []*string{&message.E164}
	req.SmsSdkAppId = &cfg.SDKAppID
	req.SignName = &cfg.SignName
	req.TemplateId = &templateID
	req.TemplateParamSet = []*string{&message.Content}
	return req
}
