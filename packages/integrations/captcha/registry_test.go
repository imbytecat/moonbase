package captcha

import (
	"context"
	"testing"

	"github.com/imbytecat/moonbase/integrations/core/config"
	"github.com/imbytecat/moonbase/integrations/core/integration"
)

type testConfig struct {
	SiteKey string `json:"siteKey" jsonschema:"required,minLength=1"`
}

func TestRegistryRejectsInvalidConfigBeforeVerify(t *testing.T) {
	called := false
	r := MustRegistry(Register("test", integration.Presentation{Name: "测试"}, config.MustContract[testConfig](config.Policy{}), Operations[testConfig]{SiteKey: func(c testConfig) string { return c.SiteKey }, Verify: func(context.Context, testConfig, string, string) error { called = true; return nil }}))
	err := r.Verify(t.Context(), "test", map[string]any{}, "token", "")
	if err == nil {
		t.Fatal("invalid config should fail closed")
	}
	if called {
		t.Fatal("provider must not run")
	}
}
