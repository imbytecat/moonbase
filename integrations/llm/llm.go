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

	"github.com/imbytecat/moonbase/server/integrationkit/integration"
	kitsettings "github.com/imbytecat/moonbase/server/integrationkit/settings"
	"github.com/imbytecat/moonbase/server/integrationkit/systemcodec"
)

// AI purposes are code, not data: each is a fixed slot the application calls
// models through, and operators bind each one to a connection profile. Adding
// a feature that talks to a model = adding a purpose here.
const (
	// PurposeChat is the general chat/completion slot used by application
	// features that need a conversational model.
	PurposeChat = "chat"
)

// Purposes is the catalog served to the admin UI, in display order.
var Purposes = integration.Catalog{PurposeChat}

// ErrNotConfigured signals that the purpose is unbound or its profile is
// incomplete; callers map it to a friendly "not configured" RPC error.
var ErrNotConfigured = fmt.Errorf("ai model is not configured")

type Config = kitsettings.Integration[systemcodec.LlmProfile]

type Loader func(ctx context.Context) (Config, error)

// Chatter is the semantic seam business code depends on: one prompt in, one
// completion out, addressed by purpose. Streaming/tool use can extend this
// interface when a real feature needs it.
type Chatter interface {
	Complete(ctx context.Context, purpose, systemPrompt, userPrompt string) (string, error)
	CompleteWith(ctx context.Context, profile systemcodec.LlmProfile, systemPrompt, userPrompt string) (string, error)
}

type completeFunc = func(ctx context.Context, p systemcodec.LlmProfile, systemPrompt, userPrompt string) (string, error)

var drivers = integration.Registry[systemcodec.LlmProfile, completeFunc]{
	"openai": {
		Usable: func(p systemcodec.LlmProfile) bool {
			return p.Openai.ApiKey != "" && p.Openai.Model != ""
		},
		Ops: completeOpenAI,
	},
	"anthropic": {
		Usable: func(p systemcodec.LlmProfile) bool {
			return p.Anthropic.ApiKey != "" && p.Anthropic.Model != ""
		},
		Ops: completeAnthropic,
	},
}

// Providers lists registered driver names, sorted.
func Providers() []string {
	return drivers.Names()
}

// ProfileUsable reports whether the profile's driver is fully configured —
// the same gate CompleteWith enforces.
func ProfileUsable(p systemcodec.LlmProfile) bool {
	return drivers.ProfileUsable(p)
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

func (c *Client) CompleteWith(ctx context.Context, profile systemcodec.LlmProfile, systemPrompt, userPrompt string) (string, error) {
	complete, ok := drivers.OpsFor(profile)
	if !ok {
		return "", ErrNotConfigured
	}
	return complete(ctx, profile, systemPrompt, userPrompt)
}

func completeOpenAI(ctx context.Context, p systemcodec.LlmProfile, systemPrompt, userPrompt string) (string, error) {
	opts := []openaioption.RequestOption{openaioption.WithAPIKey(p.Openai.ApiKey)}
	if p.Openai.BaseUrl != "" {
		opts = append(opts, openaioption.WithBaseURL(p.Openai.BaseUrl))
	}
	client := openai.NewClient(opts...)

	messages := []openai.ChatCompletionMessageParamUnion{}
	if systemPrompt != "" {
		messages = append(messages, openai.SystemMessage(systemPrompt))
	}
	messages = append(messages, openai.UserMessage(userPrompt))

	resp, err := client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Model:    p.Openai.Model,
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

func completeAnthropic(ctx context.Context, p systemcodec.LlmProfile, systemPrompt, userPrompt string) (string, error) {
	opts := []anthropicoption.RequestOption{anthropicoption.WithAPIKey(p.Anthropic.ApiKey)}
	if p.Anthropic.BaseUrl != "" {
		opts = append(opts, anthropicoption.WithBaseURL(p.Anthropic.BaseUrl))
	}
	client := anthropic.NewClient(opts...)

	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(p.Anthropic.Model),
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
