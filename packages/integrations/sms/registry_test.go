package sms

import (
	"context"
	"testing"

	"github.com/imbytecat/moonbase/integrations/core/config"
	"github.com/imbytecat/moonbase/integrations/core/integration"
)

type testConfig struct {
	Endpoint string `json:"endpoint" jsonschema:"required,minLength=1"`
	Secret   string `json:"secret" jsonschema:"required,minLength=1"`
}

func TestRegistryRejectsInvalidConfigBeforeSending(t *testing.T) {
	called := false
	registry := MustRegistry(Register(
		"test",
		integration.Presentation{Name: "测试短信"},
		config.MustContract[testConfig](config.Policy{Secrets: []string{"/secret"}}),
		func(context.Context, testConfig, Message) error {
			called = true
			return nil
		},
	))

	err := registry.SendTemplate(t.Context(), "test", map[string]any{"endpoint": "https://example.com"}, "SMS_1", "+8613800138000", "123456")
	if err == nil {
		t.Fatal("invalid config should be rejected")
	}
	if called {
		t.Fatal("provider operation must not run for invalid config")
	}
}
