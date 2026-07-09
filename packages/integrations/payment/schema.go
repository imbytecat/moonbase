package pay

import "github.com/imbytecat/moonbase/packages/integrations/core/schema"

var (
	alipaySchema = schema.Schema{Fields: []schema.Field{
		{Key: "appId", Label: "App ID", Type: schema.String, Required: true, MaxLen: 32},
		{Key: "appPrivateKey", Label: "App private key", Type: schema.Text, Secret: true, Required: true, MaxLen: 8192},
		{Key: "authMethod", Label: "Auth method", Type: schema.Enum, Options: []string{"", AuthPublicKey, AuthCert}},
		{Key: "alipayPublicKey", Label: "Alipay public key", Type: schema.Text, MaxLen: 8192},
		{Key: "appCert", Label: "App cert", Type: schema.Text, MaxLen: 16384},
		{Key: "alipayRootCert", Label: "Alipay root cert", Type: schema.Text, MaxLen: 16384},
		{Key: "alipayPublicCert", Label: "Alipay public cert", Type: schema.Text, MaxLen: 16384},
		{Key: "opAppId", Label: "Mini-program App ID", Type: schema.String, MaxLen: 32},
		{Key: "methods", Label: "Signed products", Type: schema.Strings, Options: providerMethods("alipay"), MaxLen: 32, Unique: true},
	}}

	wechatSchema = schema.Schema{Fields: []schema.Field{
		{Key: "mchId", Label: "Merchant ID", Type: schema.String, Required: true, MaxLen: 32},
		{Key: "appId", Label: "App ID", Type: schema.String, Required: true, MaxLen: 32},
		{Key: "mchCertSerialNo", Label: "Merchant cert serial no", Type: schema.String, Required: true, MaxLen: 64},
		{Key: "mchPrivateKey", Label: "Merchant private key", Type: schema.Text, Secret: true, Required: true, MaxLen: 8192},
		{Key: "apiV3Key", Label: "APIv3 key", Type: schema.String, Secret: true, Required: true, MaxLen: 64},
		{Key: "authMethod", Label: "Auth method", Type: schema.Enum, Options: []string{"", AuthPublicKey, AuthPlatformCert}},
		{Key: "publicKeyId", Label: "Public key ID", Type: schema.String, MaxLen: 64},
		{Key: "publicKey", Label: "Public key", Type: schema.Text, MaxLen: 8192},
		{Key: "methods", Label: "Signed products", Type: schema.Strings, Options: providerMethods("wechat"), MaxLen: 32, Unique: true},
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
