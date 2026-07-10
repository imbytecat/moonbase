package pay_test

import (
	"context"
	"slices"
	"testing"

	"github.com/imbytecat/moonbase/integrations/core/config"
	"github.com/imbytecat/moonbase/integrations/core/integration"
	. "github.com/imbytecat/moonbase/integrations/payment"
)

type registryConfig struct {
	Endpoint string `json:"endpoint" jsonschema:"required,minLength=1"`
	APIKey   string `json:"apiKey"   jsonschema:"required,minLength=1"`
}

func testRegistration(
	create func(context.Context, registryConfig, CreateRequest) (Action, error),
) Registration {
	return Register("test", integration.Presentation{Name: "测试支付"},
		config.MustContract[registryConfig](config.Policy{Secrets: []string{"/apiKey"}}),
		ProviderDescriptor{}, create)
}

func TestUnknownProviderNotUsable(t *testing.T) {
	if testRegistry().ConfigUsable("paypal", nil) {
		t.Error("unregistered provider should not be usable")
	}
}

func TestRegistryRejectsInvalidConfigBeforeExecution(t *testing.T) {
	called := false
	registry := MustRegistry(
		testRegistration(
			func(_ context.Context, cfg registryConfig, _ CreateRequest) (Action, error) {
				called = cfg.APIKey != ""
				return Action{}, nil
			},
		),
	)
	_, err := registry.Create(t.Context(), "test", map[string]any{
		"endpoint": "https://pay.example.test", "apiKey": "secret", "extra": true,
	}, CreateRequest{})
	if err == nil || called {
		t.Fatalf("invalid config must fail before execution: err=%v called=%v", err, called)
	}
}

func TestRegistryProjectsAndKeepsSecrets(t *testing.T) {
	registry := MustRegistry(
		testRegistration(
			func(context.Context, registryConfig, CreateRequest) (Action, error) { return Action{}, nil },
		),
	)
	created, err := registry.CreateConfig(
		"test",
		map[string]any{"endpoint": "https://pay.example.test"},
		map[string]string{"/apiKey": "secret"},
	)
	if err != nil {
		t.Fatal(err)
	}
	updated, err := registry.UpdateConfig(
		"test",
		map[string]any{"endpoint": "https://new.example.test"},
		nil,
		created,
	)
	if err != nil {
		t.Fatal(err)
	}
	view, valid := registry.ViewConfig("test", updated)
	if !valid || view.Values["apiKey"] != nil ||
		!slices.Equal(view.SetSecretPaths, []string{"/apiKey"}) {
		t.Fatalf("unsafe config view: %+v, valid=%v", view, valid)
	}
}

func TestRegistryDispatchesTypedOperations(t *testing.T) {
	var received registryConfig
	registry := MustRegistry(RegisterOperations(
		"test",
		integration.Presentation{Name: "测试支付"},
		config.MustContract[registryConfig](
			config.Policy{Secrets: []string{"/apiKey"}},
		),
		ProviderDescriptor{},
		Operations[registryConfig]{
			Plan:   func(context.Context, registryConfig, PlanRequest) (PlanResult, error) { return PlanResult{}, nil },
			Create: func(context.Context, registryConfig, CreateRequest) (Action, error) { return Action{}, nil },
			Query: func(_ context.Context, cfg registryConfig, _ string) (QueryResult, error) {
				received = cfg
				return QueryResult{Exists: true}, nil
			},
		},
	))
	values := map[string]any{"endpoint": "https://pay.example.test", "apiKey": "secret"}
	if _, err := registry.Query(t.Context(), "test", values, "order-1"); err != nil {
		t.Fatal(err)
	}
	if received.Endpoint == "" || received.APIKey == "" {
		t.Fatalf("typed config = %+v", received)
	}
}

func TestProviderContractAndDescriptor(t *testing.T) {
	registry := testRegistry()
	descriptors := registry.Descriptors()
	if len(descriptors) != 2 || descriptors[0].Key != "alipay" {
		t.Fatalf("descriptors = %+v", descriptors)
	}
	descriptors[0].Payment.Products[0].ID = "mutated"
	descriptor, _ := registry.Describe("alipay")
	if descriptor.Products[0].ID == "mutated" {
		t.Fatal("descriptor projection must not mutate registry")
	}
	_, err := registry.CreateConfig("alipay", map[string]any{
		"appId": "2021000000000000", "authMethod": AuthPublicKey, "products": []any{"unknown"},
	}, map[string]string{"/appPrivateKey": "private", "/alipayPublicKey": "public"})
	if err == nil {
		t.Fatal("provider contract must reject an unknown product")
	}
}

func TestRegistryDerivesActionRecoveryCapability(t *testing.T) {
	registry := MustRegistry(RegisterOperations(
		"test",
		integration.Presentation{Name: "测试支付"},
		config.MustContract[registryConfig](
			config.Policy{Secrets: []string{"/apiKey"}},
		),
		ProviderDescriptor{},
		Operations[registryConfig]{
			Plan:          func(context.Context, registryConfig, PlanRequest) (PlanResult, error) { return PlanResult{}, nil },
			Create:        func(context.Context, registryConfig, CreateRequest) (Action, error) { return Action{}, nil },
			Query:         func(context.Context, registryConfig, string) (QueryResult, error) { return QueryResult{}, nil },
			RecoverAction: func(context.Context, registryConfig, string) (Action, error) { return Action{Wait: &WaitAction{}}, nil },
		},
	))
	descriptor, _ := registry.Describe("test")
	if !slices.Contains(descriptor.Capabilities, "action_recovery") {
		t.Fatalf("capabilities = %v", descriptor.Capabilities)
	}
}
