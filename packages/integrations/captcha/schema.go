package captcha

import "github.com/imbytecat/moonbase/integrations/core/config"

var (
	turnstileSchema = config.Schema{Fields: []config.Field{
		{Key: "siteKey", Label: "站点密钥", Type: config.String, Required: true, MaxLen: 128},
		{Key: "secretKey", Label: "服务端密钥", Type: config.String, Secret: true, Required: true, MaxLen: 128},
	}}

	geetestSchema = config.Schema{Fields: []config.Field{
		{Key: "captchaId", Label: "验证 ID", Type: config.String, Required: true, MaxLen: 128},
		{Key: "captchaKey", Label: "验证密钥", Type: config.String, Secret: true, Required: true, MaxLen: 128},
	}}

	altchaSchema = config.Schema{Fields: []config.Field{
		{Key: "difficulty", Label: "难度", Type: config.Int, Min: 0, Max: 10000000},
	}}
)
