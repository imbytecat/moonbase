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
		{Key: "encryption", Label: "加密方式", Type: schema.Enum, Options: []schema.Option{
			{Value: "starttls", Label: "STARTTLS", Description: "常用 587 端口，先明文连接再升级为加密；留空即默认此项"},
			{Value: "ssl", Label: "SSL/TLS", Description: "常用 465 端口，全程加密"},
			{Value: "none", Label: "不加密", Description: "明文传输，仅用于本地调试"},
		}},
	}}

	cloudflareSchema = schema.Schema{Fields: []schema.Field{
		{Key: "fromAddress", Label: "发件地址", Type: schema.String, Required: true, MaxLen: 254},
		{Key: "fromName", Label: "发件人名称", Type: schema.String, MaxLen: 100},
		{Key: "accountId", Label: "账户 ID", Type: schema.String, Required: true, MaxLen: 64},
		{Key: "apiToken", Label: "API 令牌", Type: schema.String, Secret: true, Required: true, MaxLen: 256},
	}}
)
