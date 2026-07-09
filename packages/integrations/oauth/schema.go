package oauth

import "github.com/imbytecat/moonbase/integrations/core/schema"

var (
	oidcSchema = schema.Schema{Fields: []schema.Field{
		{Key: "key", Label: "标识", Type: schema.String, Immutable: true, Required: true, MaxLen: 32, Pattern: "^[a-z][a-z0-9-]{1,31}$", Help: "用于身份记录和登录跳转地址，创建后不可修改"},
		{Key: "issuer", Label: "签发方地址", Type: schema.String, Required: true, MaxLen: 512},
		{Key: "clientId", Label: "客户端 ID", Type: schema.String, Required: true, MaxLen: 256},
		{Key: "clientSecret", Label: "客户端 Secret", Type: schema.String, Secret: true, Required: true, MaxLen: 256},
		{Key: "scopes", Label: "授权范围", Type: schema.String, MaxLen: 256},
	}}

	wechatSchema = schema.Schema{Fields: []schema.Field{
		{Key: "key", Label: "标识", Type: schema.String, Immutable: true, Required: true, MaxLen: 32, Pattern: "^[a-z][a-z0-9-]{1,31}$", Help: "用于身份记录和登录跳转地址，创建后不可修改"},
		{Key: "appId", Label: "应用 ID", Type: schema.String, Required: true, MaxLen: 64},
		{Key: "appSecret", Label: "应用 Secret", Type: schema.String, Secret: true, Required: true, MaxLen: 128},
	}}
)
