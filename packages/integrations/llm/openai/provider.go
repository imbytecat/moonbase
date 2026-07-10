package openai

import (
	"context"
	"fmt"

	"github.com/imbytecat/moonbase/integrations/core/config"
	"github.com/imbytecat/moonbase/integrations/core/integration"
	llmint "github.com/imbytecat/moonbase/integrations/llm"
	sdk "github.com/openai/openai-go/v3"
	option "github.com/openai/openai-go/v3/option"
)

type providerConfig struct {
	BaseURL string `json:"baseUrl,omitempty" jsonschema:"title=接口地址,maxLength=512"`
	APIKey  string `json:"apiKey" jsonschema:"required,title=API 密钥,minLength=1,maxLength=256"`
	Model   string `json:"model" jsonschema:"required,title=模型,minLength=1,maxLength=128"`
}

func New() llmint.Registration {
	return llmint.Register("openai", integration.Presentation{Name: "OpenAI 兼容模型", Description: "连接兼容 OpenAI 协议的对话模型", Color: "#10a37f", IconRef: "antd:OpenAIOutlined"}, config.MustContract[providerConfig](config.Policy{Secrets: []string{"/apiKey"}}), complete)
}
func complete(ctx context.Context, c providerConfig, p llmint.Prompt) (string, error) {
	opts := []option.RequestOption{option.WithAPIKey(c.APIKey)}
	if c.BaseURL != "" {
		opts = append(opts, option.WithBaseURL(c.BaseURL))
	}
	client := sdk.NewClient(opts...)
	messages := []sdk.ChatCompletionMessageParamUnion{}
	if p.System != "" {
		messages = append(messages, sdk.SystemMessage(p.System))
	}
	messages = append(messages, sdk.UserMessage(p.User))
	resp, err := client.Chat.Completions.New(ctx, sdk.ChatCompletionNewParams{Model: c.Model, Messages: messages})
	if err != nil {
		return "", fmt.Errorf("chat completion: %w", err)
	}
	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("chat completion: empty response")
	}
	return resp.Choices[0].Message.Content, nil
}
