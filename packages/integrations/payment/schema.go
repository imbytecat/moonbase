package pay

import "github.com/imbytecat/moonbase/integrations/core/schema"

var (
	alipaySchema = schema.Schema{Fields: []schema.Field{
		{Key: "appId", Label: "应用 ID", Type: schema.String, Required: true, MaxLen: 32},
		{Key: "appPrivateKey", Label: "应用私钥", Type: schema.Text, Secret: true, Required: true, MaxLen: 8192},
		{Key: "authMethod", Label: "验签方式", Type: schema.Enum, Options: []string{"", AuthPublicKey, AuthCert}},
		{Key: "alipayPublicKey", Label: "支付宝公钥", Type: schema.Text, MaxLen: 8192},
		{Key: "appCert", Label: "应用公钥证书", Type: schema.Text, MaxLen: 16384},
		{Key: "alipayRootCert", Label: "支付宝根证书", Type: schema.Text, MaxLen: 16384},
		{Key: "alipayPublicCert", Label: "支付宝公钥证书", Type: schema.Text, MaxLen: 16384},
		{Key: "opAppId", Label: "小程序 APPID", Type: schema.String, MaxLen: 32},
		{Key: "methods", Label: "已签约支付产品", Type: schema.Strings, Options: providerMethods("alipay"), MaxLen: 32, Unique: true},
	}}

	wechatSchema = schema.Schema{Fields: []schema.Field{
		{Key: "mchId", Label: "商户号", Type: schema.String, Required: true, MaxLen: 32},
		{Key: "appId", Label: "应用 ID", Type: schema.String, Required: true, MaxLen: 32},
		{Key: "mchCertSerialNo", Label: "商户证书序列号", Type: schema.String, Required: true, MaxLen: 64},
		{Key: "mchPrivateKey", Label: "商户 API 私钥", Type: schema.Text, Secret: true, Required: true, MaxLen: 8192},
		{Key: "apiV3Key", Label: "APIv3 密钥", Type: schema.String, Secret: true, Required: true, MaxLen: 64},
		{Key: "authMethod", Label: "验签方式", Type: schema.Enum, Options: []string{"", AuthPublicKey, AuthPlatformCert}},
		{Key: "publicKeyId", Label: "微信支付公钥 ID", Type: schema.String, MaxLen: 64},
		{Key: "publicKey", Label: "微信支付公钥", Type: schema.Text, MaxLen: 8192},
		{Key: "methods", Label: "已签约支付产品", Type: schema.Strings, Options: providerMethods("wechat"), MaxLen: 32, Unique: true},
	}}
)

func providerMethods(provider string) []string {
	methods := methodCatalog(provider)
	out := make([]string, len(methods))
	for i, method := range methods {
		out[i] = method.ID
	}
	return out
}
