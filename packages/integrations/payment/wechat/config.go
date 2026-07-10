package wechat

import (
	"context"

	"github.com/imbytecat/moonbase/integrations/core/config"
	"github.com/imbytecat/moonbase/integrations/core/form"
	"github.com/imbytecat/moonbase/integrations/core/integration"
	pay "github.com/imbytecat/moonbase/integrations/payment"
	invopop "github.com/invopop/jsonschema"
)

var descriptor = pay.ProviderDescriptor{
	Methods: []pay.PaymentMethodDescriptor{{Key: "wechat", Presentation: integration.Presentation{
		Name: "微信支付", Description: "使用微信支付完成付款", Color: "#07c160", IconRef: "antd:WechatOutlined",
	}}},
	Products: []pay.ProductDescriptor{
		{
			ID:           "native",
			Method:       "wechat",
			Presentation: integration.Presentation{Name: "Native 支付", Description: "由付款人扫码完成支付"},
		},
		{
			ID:           "h5",
			Method:       "wechat",
			Presentation: integration.Presentation{Name: "H5 支付", Description: "跳转到移动端支付页面"},
		},
		{
			ID:           "jsapi",
			Method:       "wechat",
			Presentation: integration.Presentation{Name: "JSAPI 支付", Description: "在微信内唤起支付"},
			Input: form.Schema{
				Fields: []form.Field{
					{
						Key:      "payer_id",
						Label:    "用户标识",
						Type:     form.String,
						Required: true,
						MaxLen:   128,
					},
				},
			},
		},
		{
			ID:           "app",
			Method:       "wechat",
			Presentation: integration.Presentation{Name: "App 支付", Description: "在移动应用内唤起支付"},
		},
	},
}

type providerConfig struct {
	MchID           string   `json:"mchId"                 jsonschema:"required,title=商户号,minLength=1,maxLength=32"`
	AppID           string   `json:"appId"                 jsonschema:"required,title=应用 ID,minLength=1,maxLength=32"`
	MchCertSerialNo string   `json:"mchCertSerialNo"       jsonschema:"required,title=商户证书序列号,minLength=1,maxLength=64"`
	MchPrivateKey   string   `json:"mchPrivateKey"         jsonschema:"required,title=商户 API 私钥,minLength=1,maxLength=8192"`
	APIV3Key        string   `json:"apiV3Key"              jsonschema:"required,title=APIv3 密钥,minLength=1,maxLength=64"`
	AuthMethod      string   `json:"authMethod"            jsonschema:"required,title=验签方式,minLength=1"`
	PublicKeyID     string   `json:"publicKeyId,omitempty" jsonschema:"title=微信支付公钥 ID,maxLength=64"`
	PublicKey       string   `json:"publicKey,omitempty"   jsonschema:"title=微信支付公钥,minLength=1,maxLength=8192"`
	Products        []string `json:"products,omitempty"    jsonschema:"title=已签约支付产品,uniqueItems=true"`
}

func (providerConfig) JSONSchemaExtend(schema *invopop.Schema) {
	if authMethod, ok := schema.Properties.Get("authMethod"); ok {
		authMethod.OneOf = []*invopop.Schema{
			{Const: pay.AuthPublicKey, Title: "微信支付公钥模式"},
			{Const: pay.AuthPlatformCert, Title: "平台证书模式"},
		}
	}
	if products, ok := schema.Properties.Get("products"); ok && products.Items != nil {
		products.Description = "留空表示已签约全部产品"
		for _, product := range descriptor.Products {
			products.Items.OneOf = append(products.Items.OneOf, &invopop.Schema{
				Const: product.ID, Title: product.Presentation.Name,
			})
		}
	}
	schema.AllOf = append(schema.AllOf, &invopop.Schema{
		If: &invopop.Schema{Extras: map[string]any{"properties": map[string]any{
			"authMethod": map[string]any{"const": pay.AuthPublicKey},
		}}},
		Then: &invopop.Schema{Required: []string{"publicKeyId", "publicKey"}},
	})
}

func New() pay.Registration {
	return pay.RegisterOperations(
		"wechat",
		integration.Presentation{
			Name:        "微信支付",
			Description: "微信支付直连商户",
			Color:       "#07c160",
			IconRef:     "antd:WechatOutlined",
		},
		config.MustContract[providerConfig](
			config.Policy{Secrets: []string{"/mchPrivateKey", "/apiV3Key", "/publicKey"}},
		),
		descriptor,
		pay.Operations[providerConfig]{
			Plan: func(_ context.Context, cfg providerConfig, request pay.PlanRequest) (pay.PlanResult, error) {
				return pay.PlanConfigured(descriptor, "wechat", cfg.Products, request)
			},
			Create: func(ctx context.Context, cfg providerConfig, request pay.CreateRequest) (pay.Action, error) {
				return pay.CreateTyped(ctx, cfg, cfg.Products, descriptor, request, wechatCreate)
			},
			Query:       wechatQuery,
			Refund:      wechatRefund,
			QueryRefund: wechatQueryRefund,
			ParseNotify: wechatParseNotify,
			RenderHostedFlow: func(product, payload string) ([]byte, error) {
				return pay.RenderHostedFlowForProvider("wechat", product, payload)
			},
		},
	)
}
