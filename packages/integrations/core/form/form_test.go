package form

import "testing"

func TestFormValidatesConditionalFields(t *testing.T) {
	schema := Schema{Fields: []Field{
		{Key: "mode", Label: "模式", Type: Enum, Required: true, Options: []Option{{Value: "public"}, {Value: "cert"}}},
		{Key: "certificate", Label: "证书", Type: Text, Required: true, ShowWhen: &ShowWhen{Field: "mode", Values: []string{"cert"}}},
	}}
	if err := schema.Validate(map[string]any{"mode": "public"}); err != nil {
		t.Fatalf("inactive conditional field should be optional: %v", err)
	}
	if err := schema.Validate(map[string]any{"mode": "cert"}); err == nil {
		t.Fatal("active conditional field should be required")
	}
}

func TestJSONFormPreservesOrderOptionsAndConditions(t *testing.T) {
	schema := Schema{Fields: []Field{
		{Key: "mode", Label: "模式", Type: Enum, Required: true, Options: []Option{{Value: "public", Label: "公钥", Description: "使用公钥"}, {Value: "cert", Label: "证书"}}},
		{Key: "certificate", Label: "证书", Type: Text, Required: true, ShowWhen: &ShowWhen{Field: "mode", Values: []string{"cert"}}},
	}}
	jsonSchema, ui := schema.JSONForm()
	order := ui["ui:order"].([]any)
	if len(order) != 2 || order[0] != "mode" || order[1] != "certificate" {
		t.Fatalf("ui:order = %v", order)
	}
	if len(jsonSchema["allOf"].([]any)) != 1 {
		t.Fatalf("allOf = %v, want one conditional group", jsonSchema["allOf"])
	}
	descriptions := ui["mode"].(map[string]any)["ui:options"].(map[string]any)["descriptions"].(map[string]any)
	if descriptions["public"] != "使用公钥" {
		t.Fatalf("descriptions = %v", descriptions)
	}
}

func TestDefinitionRejectsDuplicateAndUnknownCondition(t *testing.T) {
	if err := (Schema{Fields: []Field{{Key: "x", Label: "一", Type: String}, {Key: "x", Label: "二", Type: String}}}).ValidateDefinition(); err == nil {
		t.Fatal("duplicate field key should fail")
	}
	if err := (Schema{Fields: []Field{{Key: "x", Label: "一", Type: String, ShowWhen: &ShowWhen{Field: "missing", Values: []string{"yes"}}}}}).ValidateDefinition(); err == nil {
		t.Fatal("unknown conditional field should fail")
	}
}
