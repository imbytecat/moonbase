package captcha

import "github.com/imbytecat/moonbase/integrations/core/schema"

var (
	turnstileSchema = schema.Schema{Fields: []schema.Field{
		{Key: "siteKey", Label: "站点密钥", Type: schema.String, Required: true, MaxLen: 128},
		{Key: "secretKey", Label: "服务端密钥", Type: schema.String, Secret: true, Required: true, MaxLen: 128},
	}}

	geetestSchema = schema.Schema{Fields: []schema.Field{
		{Key: "captchaId", Label: "验证 ID", Type: schema.String, Required: true, MaxLen: 128},
		{Key: "captchaKey", Label: "验证密钥", Type: schema.String, Secret: true, Required: true, MaxLen: 128},
	}}

	altchaSchema = schema.Schema{Fields: []schema.Field{
		{Key: "difficulty", Label: "难度", Type: schema.Int, Min: 0, Max: 10000000},
	}}
)
