package email

import "github.com/imbytecat/moonbase/packages/integrations/core/schema"

var (
	smtpSchema = schema.Schema{Fields: []schema.Field{
		{Key: "fromAddress", Label: "From address", Type: schema.String, Required: true, MaxLen: 254},
		{Key: "fromName", Label: "From name", Type: schema.String, MaxLen: 100},
		{Key: "host", Label: "Host", Type: schema.String, Required: true, MaxLen: 253},
		{Key: "port", Label: "Port", Type: schema.Int, Min: 0, Max: 65535},
		{Key: "username", Label: "Username", Type: schema.String, MaxLen: 128},
		{Key: "password", Label: "Password", Type: schema.String, Secret: true, MaxLen: 128},
		{Key: "encryption", Label: "Encryption", Type: schema.Enum, Options: []string{"", "starttls", "ssl", "none"}},
	}}

	cloudflareSchema = schema.Schema{Fields: []schema.Field{
		{Key: "fromAddress", Label: "From address", Type: schema.String, Required: true, MaxLen: 254},
		{Key: "fromName", Label: "From name", Type: schema.String, MaxLen: 100},
		{Key: "accountId", Label: "Account ID", Type: schema.String, Required: true, MaxLen: 64},
		{Key: "apiToken", Label: "API token", Type: schema.String, Secret: true, Required: true, MaxLen: 256},
	}}
)
