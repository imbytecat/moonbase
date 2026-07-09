package email

import "github.com/imbytecat/moonbase/integrations/core/schema"

var (
	smtpSchema = schema.Schema{Fields: []schema.Field{
		{Key: "fromAddress", Label: "发件地址", Type: schema.String, Required: true, MaxLen: 254},
		{Key: "fromName", Label: "发件人名称", Type: schema.String, MaxLen: 100},
		{Key: "host", Label: "服务器地址", Type: schema.String, Required: true, MaxLen: 253},
		{Key: "port", Label: "端口", Type: schema.Int, Min: 0, Max: 65535},
		{Key: "username", Label: "用户名", Type: schema.String, MaxLen: 128},
		{Key: "password", Label: "密码", Type: schema.String, Secret: true, MaxLen: 128},
		{Key: "encryption", Label: "加密方式", Type: schema.Enum, Options: []string{"", "starttls", "ssl", "none"}},
	}}

	cloudflareSchema = schema.Schema{Fields: []schema.Field{
		{Key: "fromAddress", Label: "发件地址", Type: schema.String, Required: true, MaxLen: 254},
		{Key: "fromName", Label: "发件人名称", Type: schema.String, MaxLen: 100},
		{Key: "accountId", Label: "账户 ID", Type: schema.String, Required: true, MaxLen: 64},
		{Key: "apiToken", Label: "API 令牌", Type: schema.String, Secret: true, Required: true, MaxLen: 256},
	}}
)
