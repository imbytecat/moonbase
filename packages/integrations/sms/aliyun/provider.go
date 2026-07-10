package aliyun

import (
	"context"
	"fmt"

	openapiutil "github.com/alibabacloud-go/darabonba-openapi/v2/utils"
	dysmsapi "github.com/alibabacloud-go/dysmsapi-20170525/v5/client"

	"github.com/imbytecat/moonbase/integrations/core/config"
	"github.com/imbytecat/moonbase/integrations/core/integration"
	"github.com/imbytecat/moonbase/integrations/core/phone"
	smsint "github.com/imbytecat/moonbase/integrations/sms"
)

type providerConfig struct {
	AccessKeyID     string `json:"accessKeyId" jsonschema:"required,title=访问密钥 ID,minLength=1,maxLength=128"`
	AccessKeySecret string `json:"accessKeySecret" jsonschema:"required,title=访问密钥 Secret,minLength=1,maxLength=128"`
	SignName        string `json:"signName" jsonschema:"required,title=短信签名,minLength=1,maxLength=64"`
	TemplateCode    string `json:"templateCode" jsonschema:"required,title=模板编号,description=模板需包含一个 {code} 变量,minLength=1,maxLength=64"`
}

func New() smsint.Registration {
	return smsint.Register("aliyun", integration.Presentation{
		Name: "阿里云短信", Description: "通过云短信服务发送验证码与通知", Color: "#ff6a00", IconRef: "antd:AliyunOutlined",
	}, config.MustContract[providerConfig](config.Policy{Secrets: []string{"/accessKeySecret"}}), send)
}

func send(_ context.Context, cfg providerConfig, message smsint.Message) error {
	target, templateCode, templateParam, err := requestValues(cfg, message)
	if err != nil {
		return err
	}
	endpoint := "dysmsapi.aliyuncs.com"
	client, err := dysmsapi.NewClient(&openapiutil.Config{AccessKeyId: &cfg.AccessKeyID, AccessKeySecret: &cfg.AccessKeySecret, Endpoint: &endpoint})
	if err != nil {
		return fmt.Errorf("create sms client: %w", err)
	}
	resp, err := client.SendSms(&dysmsapi.SendSmsRequest{PhoneNumbers: &target, SignName: &cfg.SignName, TemplateCode: &templateCode, TemplateParam: &templateParam})
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

func requestValues(cfg providerConfig, message smsint.Message) (string, string, string, error) {
	target := message.E164
	if phone.RegionOf(message.E164) == "CN" {
		national, err := phone.NationalNumber(message.E164)
		if err != nil {
			return "", "", "", err
		}
		target = national
	}
	templateCode := message.TemplateCode
	if templateCode == "" {
		templateCode = cfg.TemplateCode
	}
	templateParam := fmt.Sprintf(`{"code":%q}`, message.Content)
	return target, templateCode, templateParam, nil
}
