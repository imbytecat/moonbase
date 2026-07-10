package llm

import "github.com/imbytecat/moonbase/integrations/core/config"

var (
	openAISchema = config.Schema{Fields: []config.Field{
		{Key: "baseUrl", Label: "接口地址", Type: config.String, MaxLen: 512},
		{Key: "apiKey", Label: "API 密钥", Type: config.String, Secret: true, Required: true, MaxLen: 256},
		{Key: "model", Label: "模型", Type: config.String, Required: true, MaxLen: 128},
	}}

	anthropicSchema = config.Schema{Fields: []config.Field{
		{Key: "baseUrl", Label: "接口地址", Type: config.String, MaxLen: 512},
		{Key: "apiKey", Label: "API 密钥", Type: config.String, Secret: true, Required: true, MaxLen: 256},
		{Key: "model", Label: "模型", Type: config.String, Required: true, MaxLen: 128},
	}}
)
