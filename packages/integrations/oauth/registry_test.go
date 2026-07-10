package oauth_test

import (
	"context"
	"testing"

	"github.com/imbytecat/moonbase/integrations/core/config"
	"github.com/imbytecat/moonbase/integrations/core/integration"
	"github.com/imbytecat/moonbase/integrations/oauth"
)

type registryConfig struct {
	Key    string `json:"key"    jsonschema:"required,minLength=2"`
	Secret string `json:"secret" jsonschema:"required,minLength=1"`
}

func TestRegistryRejectsInvalidConfigBeforeAuthorize(t *testing.T) {
	called := false
	registry := oauth.MustRegistry(oauth.Register(
		"test",
		integration.Presentation{Name: "测试登录"},
		config.MustContract[registryConfig](
			config.Policy{Secrets: []string{"/secret"}, CreateOnly: []string{"/key"}},
		),
		oauth.Operations[registryConfig]{
			AuthorizeURL: func(context.Context, registryConfig, string, string) (string, oauth.FlowSecrets, error) {
				called = true
				return "", oauth.FlowSecrets{}, nil
			},
			Exchange: func(context.Context, registryConfig, string, string, oauth.FlowSecrets) (oauth.ExternalIdentity, error) {
				return oauth.ExternalIdentity{}, nil
			},
		},
	))
	_, _, err := registry.AuthorizeURL(
		t.Context(),
		"test",
		map[string]any{"key": "x", "secret": "s", "extra": true},
		"https://app/callback",
		"state",
	)
	if err == nil || called {
		t.Fatalf("invalid config must fail before authorize: err=%v called=%v", err, called)
	}
}

func TestRegistryEnforcesCreateOnlyKeyAndWriteOnlySecret(t *testing.T) {
	registry := oauth.MustRegistry(oauth.Register(
		"test",
		integration.Presentation{Name: "测试登录"},
		config.MustContract[registryConfig](
			config.Policy{Secrets: []string{"/secret"}, CreateOnly: []string{"/key"}},
		),
		oauth.Operations[registryConfig]{
			AuthorizeURL: func(context.Context, registryConfig, string, string) (string, oauth.FlowSecrets, error) {
				return "", oauth.FlowSecrets{}, nil
			},
			Exchange: func(context.Context, registryConfig, string, string, oauth.FlowSecrets) (oauth.ExternalIdentity, error) {
				return oauth.ExternalIdentity{}, nil
			},
		},
	))
	created, err := registry.CreateConfig(
		"test",
		map[string]any{"key": "login"},
		map[string]string{"/secret": "secret"},
	)
	if err != nil {
		t.Fatal(err)
	}
	view, valid := registry.ViewConfig("test", created)
	if !valid || view.Values["secret"] != nil || len(view.SetSecretPaths) != 1 {
		t.Fatalf("view=%+v valid=%v", view, valid)
	}
	if _, err := registry.UpdateConfig(
		"test",
		map[string]any{"key": "changed"},
		nil,
		created,
	); err == nil {
		t.Fatal("create-only key must not change")
	}
	broken := map[string]any{"secret": "secret"}
	if _, err := registry.UpdateConfig(
		"test",
		map[string]any{"key": "repaired"},
		nil,
		broken,
	); err != nil {
		t.Fatalf("missing stored key must remain repairable: %v", err)
	}
}
