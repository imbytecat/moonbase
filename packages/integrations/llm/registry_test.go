package llm

import (
	"context"
	"testing"

	"github.com/imbytecat/moonbase/integrations/core/config"
	"github.com/imbytecat/moonbase/integrations/core/integration"
)

type testConfig struct {
	Model string `json:"model" jsonschema:"required,minLength=1"`
}

func TestRegistryRejectsInvalidConfigBeforeCompletion(t *testing.T) {
	called := false
	r := MustRegistry(Register("test", integration.Presentation{Name: "测试"}, config.MustContract[testConfig](config.Policy{}), func(context.Context, testConfig, Prompt) (string, error) { called = true; return "", nil }))
	_, err := r.Complete(t.Context(), "test", map[string]any{}, "", "hi")
	if err == nil {
		t.Fatal("invalid config should fail")
	}
	if called {
		t.Fatal("provider must not run")
	}
}
