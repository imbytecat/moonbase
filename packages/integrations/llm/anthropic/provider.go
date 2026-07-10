package anthropic

import (
	"context"
	"fmt"

	sdk "github.com/anthropics/anthropic-sdk-go"
	option "github.com/anthropics/anthropic-sdk-go/option"
	"github.com/imbytecat/moonbase/integrations/core/config"
	"github.com/imbytecat/moonbase/integrations/core/integration"
	llmint "github.com/imbytecat/moonbase/integrations/llm"
)

type providerConfig struct {
	BaseURL string `json:"baseUrl,omitempty" jsonschema:"title=接口地址,maxLength=512"`
	APIKey  string `json:"apiKey"            jsonschema:"required,title=API 密钥,minLength=1,maxLength=256"`
	Model   string `json:"model"             jsonschema:"required,title=模型,minLength=1,maxLength=128"`
}

func New() llmint.Registration {
	return llmint.Register(
		"anthropic",
		integration.Presentation{
			Name:        "Anthropic 模型",
			Description: "连接 Anthropic 消息协议的对话模型",
			Color:       "#d97757",
			IconRef:     "antd:AnthropicFilled",
		},
		config.MustContract[providerConfig](config.Policy{Secrets: []string{"/apiKey"}}),
		complete,
	)
}
func complete(ctx context.Context, c providerConfig, p llmint.Prompt) (string, error) {
	opts := []option.RequestOption{option.WithAPIKey(c.APIKey)}
	if c.BaseURL != "" {
		opts = append(opts, option.WithBaseURL(c.BaseURL))
	}
	client := sdk.NewClient(opts...)
	params := sdk.MessageNewParams{
		Model:     sdk.Model(c.Model),
		MaxTokens: 1024,
		Messages:  []sdk.MessageParam{sdk.NewUserMessage(sdk.NewTextBlock(p.User))},
	}
	if p.System != "" {
		params.System = []sdk.TextBlockParam{{Text: p.System}}
	}
	resp, err := client.Messages.New(ctx, params)
	if err != nil {
		return "", fmt.Errorf("message completion: %w", err)
	}
	for _, block := range resp.Content {
		if block.Type == "text" {
			return block.Text, nil
		}
	}
	return "", fmt.Errorf("message completion: no text content")
}
