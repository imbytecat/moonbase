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

	err := registry.Send(t.Context(), "test", map[string]any{"endpoint": "https://mail.example.com"}, Message{To: "user@example.com", Subject: "主题", TextBody: "正文"})
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
		if err := registry.Send(t.Context(), profile.Provider, profile.Config, Message{}); err == nil {
			t.Fatalf("Send(%+v) must fail", profile)
		}
	}
}

func TestRegistryRejectsIncompleteRegistrations(t *testing.T) {
	validContract := config.MustContract[registryTestConfig](config.Policy{})
	tests := []struct {
		name         string
		registration Registration
	}{
		{
			name: "invalid icon",
			registration: Register("bad", integration.Presentation{
				Name: "坏图标", IconRef: "CloudOutlined",
			}, validContract, func(context.Context, registryTestConfig, Message) error { return nil }),
		},
		{
			name: "zero contract",
			registration: Register("bad", integration.Presentation{
				Name: "缺少契约", IconRef: "antd:ApiOutlined",
			}, config.Contract[registryTestConfig]{}, func(context.Context, registryTestConfig, Message) error { return nil }),
		},
		{
			name: "nil operation",
			registration: Register("bad", integration.Presentation{
				Name: "缺少操作", IconRef: "antd:ApiOutlined",
			}, validContract, nil),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := NewRegistry(tt.registration); err == nil {
				t.Fatal("NewRegistry must fail")
			}
		})
	}
}
