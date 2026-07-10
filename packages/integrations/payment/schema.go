package pay

import "github.com/imbytecat/moonbase/integrations/core/schema"

var (
	authPublicKeyWhen = &schema.ShowWhen{Field: "authMethod", Values: []string{AuthPublicKey, ""}}
	authCertWhen      = &schema.ShowWhen{Field: "authMethod", Values: []string{AuthCert}}

	alipaySchema = schema.Schema{Fields: []schema.Field{
		{Key: "appId", Label: "应用 ID", Type: schema.String, Required: true, MaxLen: 32},
		{Key: "appPrivateKey", Label: "应用私钥", Type: schema.Text, Secret: true, Required: true, MaxLen: 8192},
		{Key: "authMethod", Label: "验签方式", Type: schema.Enum, Options: []schema.Option{
			{Value: AuthPublicKey, Label: "公钥模式", Description: "仅需填写支付宝公钥；留空即默认此项"},
			{Value: AuthCert, Label: "证书模式", Description: "需上传应用公钥证书、支付宝公钥证书与支付宝根证书"},
		}},
		{Key: "alipayPublicKey", Label: "支付宝公钥", Type: schema.Text, Required: true, MaxLen: 8192, ShowWhen: authPublicKeyWhen},
		{Key: "appCert", Label: "应用公钥证书", Type: schema.Text, Required: true, MaxLen: 16384, ShowWhen: authCertWhen},
		{Key: "alipayRootCert", Label: "支付宝根证书", Type: schema.Text, Required: true, MaxLen: 16384, ShowWhen: authCertWhen},
		{Key: "alipayPublicCert", Label: "支付宝公钥证书", Type: schema.Text, Required: true, MaxLen: 16384, ShowWhen: authCertWhen},
		{Key: "opAppId", Label: "小程序 APPID", Type: schema.String, MaxLen: 32},
		{Key: "methods", Label: "已签约支付产品", Type: schema.Strings, Options: providerMethods("alipay"), MaxLen: 32, Unique: true},
	}}

	wechatSchema = schema.Schema{Fields: []schema.Field{
		{Key: "mchId", Label: "商户号", Type: schema.String, Required: true, MaxLen: 32},
		{Key: "appId", Label: "应用 ID", Type: schema.String, Required: true, MaxLen: 32},
		{Key: "mchCertSerialNo", Label: "商户证书序列号", Type: schema.String, Required: true, MaxLen: 64},
		{Key: "mchPrivateKey", Label: "商户 API 私钥", Type: schema.Text, Secret: true, Required: true, MaxLen: 8192},
		{Key: "apiV3Key", Label: "APIv3 密钥", Type: schema.String, Secret: true, Required: true, MaxLen: 64},
		{Key: "authMethod", Label: "验签方式", Type: schema.Enum, Options: []schema.Option{
			{Value: AuthPublicKey, Label: "公钥模式", Description: "填写微信支付公钥与公钥 ID；留空即默认此项"},
			{Value: AuthPlatformCert, Label: "平台证书模式", Description: "使用微信支付平台证书自动下载验签"},
		}},
		{Key: "publicKeyId", Label: "微信支付公钥 ID", Type: schema.String, Required: true, MaxLen: 64, ShowWhen: authPublicKeyWhen},
		{Key: "publicKey", Label: "微信支付公钥", Type: schema.Text, Required: true, MaxLen: 8192, ShowWhen: authPublicKeyWhen},
		{Key: "methods", Label: "已签约支付产品", Type: schema.Strings, Options: providerMethods("wechat"), MaxLen: 32, Unique: true},
	}}
)

func providerMethods(provider string) []schema.Option {
	methods := methodCatalog(provider)
	out := make([]schema.Option, len(methods))
	for i, method := range methods {
		out[i] = schema.Option{Value: method.ID}
	}
	return out
}
