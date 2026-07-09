package llm

import "github.com/imbytecat/moonbase/server/integrationkit/schema"

var (
	openAISchema = schema.Schema{Fields: []schema.Field{
		{Key: "baseUrl", Label: "Base URL", Type: schema.String, MaxLen: 512},
		{Key: "apiKey", Label: "API key", Type: schema.String, Secret: true, Required: true, MaxLen: 256},
		{Key: "model", Label: "Model", Type: schema.String, Required: true, MaxLen: 128},
	}}

	anthropicSchema = schema.Schema{Fields: []schema.Field{
		{Key: "baseUrl", Label: "Base URL", Type: schema.String, MaxLen: 512},
		{Key: "apiKey", Label: "API key", Type: schema.String, Secret: true, Required: true, MaxLen: 256},
		{Key: "model", Label: "Model", Type: schema.String, Required: true, MaxLen: 128},
	}}
)
