package alipay

import (
	"context"

	"github.com/imbytecat/moonbase/integrations/core/config"
	"github.com/imbytecat/moonbase/integrations/core/form"
	"github.com/imbytecat/moonbase/integrations/core/integration"
	pay "github.com/imbytecat/moonbase/integrations/payment"
	invopop "github.com/invopop/jsonschema"
)

var descriptor = pay.ProviderDescriptor{
	Methods: []pay.PaymentMethodDescriptor{{Key: "alipay", Presentation: integration.Presentation{
		Name: "支付宝", Description: "使用支付宝完成付款", Color: "#1677ff", IconRef: "antd:AlipayCircleOutlined",
	}}},
	Products: []pay.ProductDescriptor{
		{ID: "precreate", Method: "alipay", Presentation: integration.Presentation{Name: "当面付二维码", Description: "由付款人扫码完成支付"}},
		{ID: "page_pay", Method: "alipay", Presentation: integration.Presentation{Name: "电脑网站支付", Description: "跳转到电脑端支付页面"}},
		{ID: "wap_pay", Method: "alipay", Presentation: integration.Presentation{Name: "手机网站支付", Description: "跳转到移动端支付页面"}},
		{ID: "create", Method: "alipay", Presentation: integration.Presentation{Name: "小程序支付", Description: "在小程序内唤起支付"}, Input: form.Schema{Fields: []form.Field{{Key: "payer_id", Label: "买家标识", Type: form.String, Required: true, MaxLen: 128}}}},
		{ID: "app_pay", Method: "alipay", Presentation: integration.Presentation{Name: "App 支付", Description: "在移动应用内唤起支付"}},
	},
}

type providerConfig struct {
	AppID            string   `json:"appId" jsonschema:"required,title=应用 ID,minLength=1,maxLength=32"`
	AppPrivateKey    string   `json:"appPrivateKey" jsonschema:"required,title=应用私钥,minLength=1,maxLength=8192"`
	AuthMethod       string   `json:"authMethod" jsonschema:"required,title=验签方式,minLength=1"`
	AlipayPublicKey  string   `json:"alipayPublicKey,omitempty" jsonschema:"title=支付宝公钥,minLength=1,maxLength=8192"`
	AppCert          string   `json:"appCert,omitempty" jsonschema:"title=应用公钥证书,minLength=1,maxLength=16384"`
	AlipayRootCert   string   `json:"alipayRootCert,omitempty" jsonschema:"title=支付宝根证书,minLength=1,maxLength=16384"`
	AlipayPublicCert string   `json:"alipayPublicCert,omitempty" jsonschema:"title=支付宝公钥证书,minLength=1,maxLength=16384"`
	OpAppID          string   `json:"opAppId,omitempty" jsonschema:"title=小程序 APPID,maxLength=32"`
	Products         []string `json:"products,omitempty" jsonschema:"title=已签约支付产品,uniqueItems=true"`
}

func (providerConfig) JSONSchemaExtend(schema *invopop.Schema) {
	if authMethod, ok := schema.Properties.Get("authMethod"); ok {
		authMethod.Enum = []any{pay.AuthPublicKey, pay.AuthCert}
	}
	if products, ok := schema.Properties.Get("products"); ok && products.Items != nil {
		for _, product := range descriptor.Products {
			products.Items.Enum = append(products.Items.Enum, product.ID)
		}
	}
	schema.AllOf = append(schema.AllOf,
		requiredWhen("authMethod", pay.AuthPublicKey, "alipayPublicKey"),
		requiredWhen("authMethod", pay.AuthCert, "appCert", "alipayRootCert", "alipayPublicCert"),
	)
}

func requiredWhen(field, value string, required ...string) *invopop.Schema {
	return &invopop.Schema{If: &invopop.Schema{Extras: map[string]any{
		"properties": map[string]any{field: map[string]any{"const": value}},
	}}, Then: &invopop.Schema{Required: required}}
}

func New() pay.Registration {
	return pay.RegisterOperations(
		"alipay",
		integration.Presentation{Name: "支付宝", Description: "支付宝开放平台直连商户", Color: "#1677ff", IconRef: "antd:AlipayCircleOutlined"},
		config.MustContract[providerConfig](config.Policy{Secrets: []string{
			"/appPrivateKey", "/alipayPublicKey", "/appCert", "/alipayRootCert", "/alipayPublicCert",
		}}),
		descriptor,
		pay.Operations[providerConfig]{
			Plan: func(_ context.Context, cfg providerConfig, request pay.PlanRequest) (pay.PlanResult, error) {
				return pay.PlanConfigured(descriptor, "alipay", cfg.Products, request)
			},
			Create: func(ctx context.Context, cfg providerConfig, request pay.CreateRequest) (pay.Action, error) {
				return pay.CreateTyped(ctx, cfg, cfg.Products, descriptor, request, alipayCreate)
			},
			Query: alipayQuery, Refund: alipayRefund, QueryRefund: alipayQueryRefund,
			ParseNotify: alipayParseNotify,
			RenderHostedFlow: func(product, payload string) ([]byte, error) {
				return pay.RenderHostedFlowForProvider("alipay", product, payload)
			},
		},
	)
}
