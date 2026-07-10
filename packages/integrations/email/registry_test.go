package email

import (
	"context"
	"testing"

	"github.com/imbytecat/moonbase/integrations/core/config"
	"github.com/imbytecat/moonbase/integrations/core/integration"
	kitsettings "github.com/imbytecat/moonbase/integrations/core/settings"
)

type registryTestConfig struct {
	Endpoint string `json:"endpoint" jsonschema:"required,minLength=1"`
}

func TestRegistryDecodesTypedConfigBeforeSending(t *testing.T) {
	var received registryTestConfig
	registry := MustRegistry(Register(
		"test",
		integration.Presentation{Name: "测试邮件"},
		config.MustContract[registryTestConfig](config.Policy{}),
		func(_ context.Context, cfg registryTestConfig, _ Message) error {
			received = cfg
			return nil
		},
	))

	err := registry.Send(t.Context(), kitsettings.GenericProfile{
		Provider: "test",
		Config:   map[string]any{"endpoint": "https://mail.example.com"},
	}, Message{To: "user@example.com", Subject: "主题", TextBody: "正文"})
	if err != nil {
		t.Fatal(err)
	}
	if received.Endpoint != "https://mail.example.com" {
		t.Fatalf("typed config = %+v", received)
	}

	for _, profile := range []kitsettings.GenericProfile{
		{Provider: "missing", Config: map[string]any{}},
		{Provider: "test", Config: map[string]any{"endpoint": "", "unknown": true}},
	} {
		if err := registry.Send(t.Context(), profile, Message{}); err == nil {
			t.Fatalf("Send(%+v) must fail", profile)
		}
	}
}
