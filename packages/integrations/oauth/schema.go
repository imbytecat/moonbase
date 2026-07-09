package oauth

import "github.com/imbytecat/moonbase/packages/integrations/core/schema"

var (
	oidcSchema = schema.Schema{Fields: []schema.Field{
		{Key: "key", Label: "Key", Type: schema.String, Immutable: true, Required: true, MaxLen: 32, Pattern: "^[a-z][a-z0-9-]{1,31}$", Help: "Stable slug used in identity records and flow URLs"},
		{Key: "issuer", Label: "Issuer", Type: schema.String, Required: true, MaxLen: 512},
		{Key: "clientId", Label: "Client ID", Type: schema.String, Required: true, MaxLen: 256},
		{Key: "clientSecret", Label: "Client secret", Type: schema.String, Secret: true, Required: true, MaxLen: 256},
		{Key: "scopes", Label: "Scopes", Type: schema.String, MaxLen: 256},
	}}

	wechatSchema = schema.Schema{Fields: []schema.Field{
		{Key: "key", Label: "Key", Type: schema.String, Immutable: true, Required: true, MaxLen: 32, Pattern: "^[a-z][a-z0-9-]{1,31}$", Help: "Stable slug used in identity records and flow URLs"},
		{Key: "appId", Label: "App ID", Type: schema.String, Required: true, MaxLen: 64},
		{Key: "appSecret", Label: "App secret", Type: schema.String, Secret: true, Required: true, MaxLen: 128},
	}}
)
