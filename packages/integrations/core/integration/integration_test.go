package integration

import (
	"slices"
	"testing"

	"github.com/imbytecat/moonbase/integrations/core/config"
	"github.com/imbytecat/moonbase/integrations/core/form"
)

func TestCatalogKnown(t *testing.T) {
	cat := Catalog{{Key: "login"}, {Key: "notify"}}
	if !cat.Known("login") {
		t.Error("Known(login) = false, want true")
	}
	if cat.Known("missing") {
		t.Error("Known(missing) = true, want false")
	}
}

func TestRegistryPreservesDescriptorOrderAndDispatchesConfigBehavior(t *testing.T) {
	reg, err := NewRegistry([]Entry[string]{
		{
			Key:          "beta",
			Presentation: Presentation{Name: "乙服务", Description: "第二个", Color: "#222222", IconRef: "antd:CloudOutlined"},
			Config: config.Schema{Fields: []config.Field{{
				Key: "token", Label: "令牌", Type: form.String, Required: true, Secret: true,
			}}},
			Ops: "beta-ops",
		},
		{
			Key:          "alpha",
			Presentation: Presentation{Name: "甲服务", Description: "第一个", IconRef: "antd:ApiOutlined"},
			Config: config.Schema{Fields: []config.Field{{
				Key: "endpoint", Label: "地址", Type: form.String, Required: true,
			}}},
			Ops: "alpha-ops",
		},
	})
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}

	if got := reg.Names(); !slices.Equal(got, []string{"beta", "alpha"}) {
		t.Errorf("Names() = %v, want declaration order [beta alpha]", got)
	}
	descriptors := reg.Descriptors()
	if len(descriptors) != 2 || descriptors[0].Key != "beta" || descriptors[0].Presentation.Name != "乙服务" {
		t.Fatalf("Descriptors() lost ordered presentation: %+v", descriptors)
	}
	if reg.ProfileUsable("beta", map[string]any{}) {
		t.Fatal("missing required token should be unusable")
	}
	if !reg.ProfileUsable("beta", map[string]any{"token": "stored"}) {
		t.Fatal("configured beta profile should be usable")
	}
	masked, ok := reg.Mask("beta", map[string]any{"token": "stored"})
	if !ok || masked["token"] != "" || masked["token_set"] != true {
		t.Fatalf("Mask() = (%v, %v), want blank token and token_set", masked, ok)
	}
	ops, ok := reg.OpsFor("alpha", map[string]any{"endpoint": "https://example.test"})
	if !ok || ops != "alpha-ops" {
		t.Fatalf("OpsFor() = (%q, %v), want alpha-ops", ops, ok)
	}
}

func TestRegistryRejectsInvalidEntries(t *testing.T) {
	validConfig := config.Schema{Fields: []config.Field{{
		Key: "endpoint", Label: "地址", Type: form.String, Required: true,
	}}}
	cases := []struct {
		name    string
		entries []Entry[string]
	}{
		{"duplicate key", []Entry[string]{{Key: "same", Presentation: Presentation{Name: "一", IconRef: "antd:ApiOutlined"}, Config: validConfig, Ops: "one"}, {Key: "same", Presentation: Presentation{Name: "二", IconRef: "antd:ApiOutlined"}, Config: validConfig, Ops: "two"}}},
		{"invalid icon ref", []Entry[string]{{Key: "bad", Presentation: Presentation{Name: "坏", IconRef: "CloudOutlined"}, Config: validConfig, Ops: "ops"}}},
		{"missing presentation", []Entry[string]{{Key: "bad", Config: validConfig, Ops: "ops"}}},
		{"missing config", []Entry[string]{{Key: "bad", Presentation: Presentation{Name: "坏", IconRef: "antd:ApiOutlined"}, Ops: "ops"}}},
		{"missing ops", []Entry[string]{{Key: "bad", Presentation: Presentation{Name: "坏", IconRef: "antd:ApiOutlined"}, Config: validConfig}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := NewRegistry(tc.entries); err == nil {
				t.Fatal("NewRegistry() error = nil, want validation failure")
			}
		})
	}
}
