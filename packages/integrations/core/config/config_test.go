package config

import "testing"

func testSchema() Schema {
	return Schema{Fields: []Field{
		{Key: "endpoint", Label: "地址", Type: String, Required: true},
		{Key: "secret", Label: "密钥", Type: String, Secret: true, Required: true},
		{Key: "key", Label: "标识", Type: String, Immutable: true},
		{Key: "mode", Label: "模式", Type: Enum, Options: []Option{{Value: "a"}, {Value: "b"}}},
	}}
}

func TestMaskAndMergeKeepSecretsWriteOnly(t *testing.T) {
	schema := testSchema()
	stored := map[string]any{"endpoint": "https://example.test", "secret": "stored", "key": "stable"}
	masked := schema.Mask(stored)
	if masked["secret"] != "" || masked["secret_set"] != true {
		t.Fatalf("Mask() = %v, want blank secret with stored flag", masked)
	}
	merged := schema.Merge(map[string]any{
		"endpoint": "https://new.example.test", "secret": "", "secret_set": true, "key": "changed",
	}, stored)
	if merged["secret"] != "stored" || merged["key"] != "stable" {
		t.Fatalf("Merge() = %v, want stored secret and immutable key", merged)
	}
	if _, ok := merged["secret_set"]; ok {
		t.Fatal("read-only secret_set companion must not persist")
	}
}

func TestValidateAndUsableFollowConfigContract(t *testing.T) {
	schema := testSchema()
	valid := map[string]any{"endpoint": "https://example.test", "secret": "stored", "mode": "a"}
	if err := schema.Validate(valid); err != nil {
		t.Fatalf("Validate(valid) = %v", err)
	}
	if !schema.Usable(valid) {
		t.Fatal("valid config should be usable")
	}
	if schema.Usable(map[string]any{"endpoint": "https://example.test"}) {
		t.Fatal("missing required secret should be unusable")
	}
	if err := schema.Validate(map[string]any{"endpoint": "x", "secret": "y", "mode": "unknown"}); err == nil {
		t.Fatal("unknown enum option should fail")
	}
	if err := schema.Validate(map[string]any{"endpoint": "x", "secret": "y", "secret_set": true}); err == nil {
		t.Fatal("read-only companion should fail validation")
	}
}

func TestJSONFormAddsSecretAndImmutableUIHints(t *testing.T) {
	_, ui := testSchema().JSONForm()
	secret := ui["secret"].(map[string]any)
	if secret["ui:widget"] != "secret" || secret["ui:options"].(map[string]any)["secret"] != true {
		t.Fatalf("secret ui = %v", secret)
	}
	key := ui["key"].(map[string]any)
	if key["ui:options"].(map[string]any)["immutable"] != true {
		t.Fatalf("immutable ui = %v", key)
	}
}
