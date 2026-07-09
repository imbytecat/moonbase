package llm

import "github.com/imbytecat/moonbase/integrations/core/schema"

var (
	openAISchema = schema.Schema{Fields: []schema.Field{
		{Key: "baseUrl", Label: "接口地址", Type: schema.String, MaxLen: 512},
		{Key: "apiKey", Label: "API 密钥", Type: schema.String, Secret: true, Required: true, MaxLen: 256},
		{Key: "model", Label: "模型", Type: schema.String, Required: true, MaxLen: 128},
	}}

	anthropicSchema = schema.Schema{Fields: []schema.Field{
		{Key: "baseUrl", Label: "接口地址", Type: schema.String, MaxLen: 512},
		{Key: "apiKey", Label: "API 密钥", Type: schema.String, Secret: true, Required: true, MaxLen: 256},
		{Key: "model", Label: "模型", Type: schema.String, Required: true, MaxLen: 128},
	}}
)
