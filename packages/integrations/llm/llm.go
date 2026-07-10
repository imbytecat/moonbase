// Package llm calls chat models through connection profiles configured in
// system settings. Providers are API dialects, not vendors: "openai" is any
// OpenAI-compatible endpoint (official, DeepSeek, Qwen, Ollama, vLLM, ...)
// selected via base URL; "anthropic" is the native Messages API. Profiles are
// bound to code-defined purposes; clients are built per call so config
// changes apply without a restart.
package llm

import (
	"context"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	anthropicoption "github.com/anthropics/anthropic-sdk-go/option"
	"github.com/openai/openai-go/v3"
	openaioption "github.com/openai/openai-go/v3/option"

	"github.com/imbytecat/moonbase/integrations/core/integration"
	kitsettings "github.com/imbytecat/moonbase/integrations/core/settings"
)

// ErrNotConfigured signals that the purpose is unbound or its profile is
// incomplete; callers map it to a friendly "not configured" RPC error.
var ErrNotConfigured = fmt.Errorf("ai model is not configured")

type Config = kitsettings.Integration[kitsettings.GenericProfile]

type Loader func(ctx context.Context) (Config, error)

// Chatter is the semantic seam business code depends on: one prompt in, one
// completion out, addressed by purpose. Streaming/tool use can extend this
// interface when a real feature needs it.
type Chatter interface {
	Complete(ctx context.Context, purpose, systemPrompt, userPrompt string) (string, error)
	CompleteWith(ctx context.Context, profile kitsettings.GenericProfile, systemPrompt, userPrompt string) (string, error)
}

type completeFunc = func(ctx context.Context, config map[string]any, systemPrompt, userPrompt string) (string, error)

var Registry = integration.MustRegistry([]integration.Entry[completeFunc]{
	{
		Key:          "openai",
		Presentation: integration.Presentation{Name: "OpenAI 兼容模型", Description: "连接兼容 OpenAI 协议的对话模型", Color: "#10a37f", IconRef: "antd:OpenAIOutlined"},
		Config:       openAISchema,
		Ops:          completeOpenAI,
	},
	{
		Key:          "anthropic",
		Presentation: integration.Presentation{Name: "Anthropic 模型", Description: "连接 Anthropic 消息协议的对话模型", Color: "#d97757", IconRef: "antd:RobotOutlined"},
		Config:       anthropicSchema,
		Ops:          completeAnthropic,
	},
})

func Providers() []string { return Registry.Names() }

// ProfileUsable reports whether the profile's driver is fully configured —
// the same gate CompleteWith enforces.
func ProfileUsable(p kitsettings.GenericProfile) bool {
	return Registry.ProfileUsable(p.Provider, p.Config)
}

// Usable reports whether the purpose resolves to a usable profile.
func Usable(cfg Config, purpose string) bool {
	p, ok := cfg.ProfileFor(purpose)
	return ok && ProfileUsable(p)
}

type Client struct {
	load Loader
}

func NewClient(load Loader) *Client {
	return &Client{load: load}
}

var _ Chatter = (*Client)(nil)

func (c *Client) Complete(ctx context.Context, purpose, systemPrompt, userPrompt string) (string, error) {
	cfg, err := c.load(ctx)
	if err != nil {
		return "", err
	}
	p, ok := cfg.ProfileFor(purpose)
	if !ok {
		return "", ErrNotConfigured
	}
	return c.CompleteWith(ctx, p, systemPrompt, userPrompt)
}

func (c *Client) CompleteWith(ctx context.Context, profile kitsettings.GenericProfile, systemPrompt, userPrompt string) (string, error) {
	complete, ok := Registry.OpsFor(profile.Provider, profile.Config)
	if !ok {
		return "", ErrNotConfigured
	}
	return complete(ctx, profile.Config, systemPrompt, userPrompt)
}

func completeOpenAI(ctx context.Context, config map[string]any, systemPrompt, userPrompt string) (string, error) {
	opts := []openaioption.RequestOption{openaioption.WithAPIKey(cfgStr(config, "apiKey"))}
	if baseURL := cfgStr(config, "baseUrl"); baseURL != "" {
		opts = append(opts, openaioption.WithBaseURL(baseURL))
	}
	client := openai.NewClient(opts...)

	messages := []openai.ChatCompletionMessageParamUnion{}
	if systemPrompt != "" {
		messages = append(messages, openai.SystemMessage(systemPrompt))
	}
	messages = append(messages, openai.UserMessage(userPrompt))

	resp, err := client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Model:    cfgStr(config, "model"),
		Messages: messages,
	})
	if err != nil {
		return "", fmt.Errorf("chat completion: %w", err)
	}
	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("chat completion: empty response")
	}
	return resp.Choices[0].Message.Content, nil
}

func completeAnthropic(ctx context.Context, config map[string]any, systemPrompt, userPrompt string) (string, error) {
	opts := []anthropicoption.RequestOption{anthropicoption.WithAPIKey(cfgStr(config, "apiKey"))}
	if baseURL := cfgStr(config, "baseUrl"); baseURL != "" {
		opts = append(opts, anthropicoption.WithBaseURL(baseURL))
	}
	client := anthropic.NewClient(opts...)

	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(cfgStr(config, "model")),
		MaxTokens: 1024,
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(userPrompt)),
		},
	}
	if systemPrompt != "" {
		params.System = []anthropic.TextBlockParam{{Text: systemPrompt}}
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

func cfgStr(config map[string]any, key string) string {
	s, _ := config[key].(string)
	return s
}
