package captcha

import "github.com/imbytecat/moonbase/server/integrationkit/schema"

var (
	turnstileSchema = schema.Schema{Fields: []schema.Field{
		{Key: "siteKey", Label: "Site key", Type: schema.String, Required: true, MaxLen: 128},
		{Key: "secretKey", Label: "Secret key", Type: schema.String, Secret: true, Required: true, MaxLen: 128},
	}}

	geetestSchema = schema.Schema{Fields: []schema.Field{
		{Key: "captchaId", Label: "Captcha ID", Type: schema.String, Required: true, MaxLen: 128},
		{Key: "captchaKey", Label: "Captcha key", Type: schema.String, Secret: true, Required: true, MaxLen: 128},
	}}

	altchaSchema = schema.Schema{Fields: []schema.Field{
		{Key: "difficulty", Label: "Difficulty", Type: schema.Int, MaxLen: 8},
	}}
)
