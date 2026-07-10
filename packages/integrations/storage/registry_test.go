package storage

import (
	"context"
	"testing"
	"time"

	"github.com/imbytecat/moonbase/integrations/core/config"
	"github.com/imbytecat/moonbase/integrations/core/integration"
)

type testConfig struct {
	Directory string `json:"directory" jsonschema:"required,minLength=1"`
}

func TestRegistryRejectsInvalidConfigBeforeOperation(t *testing.T) {
	called := false
	registry := MustRegistry(Register(
		"test",
		integration.Presentation{Name: "测试存储"},
		config.MustContract[testConfig](config.Policy{}),
		Operations[testConfig]{
			PresignPut: func(Runtime, context.Context, testConfig, string, string, string, time.Duration) (string, error) {
				return "", nil
			},
			ResolveURL: func(Runtime, context.Context, testConfig, string, string, time.Duration) (string, error) {
				return "", nil
			},
			Delete: func(Runtime, context.Context, testConfig, string, string) error { return nil },
			Test:   func(context.Context, Runtime, testConfig) error { called = true; return nil },
		},
	))
	err := registry.Test(t.Context(), nil, "test", map[string]any{})
	if err == nil {
		t.Fatal("invalid config should be rejected")
	}
	if called {
		t.Fatal("provider operation must not run for invalid config")
	}
}
