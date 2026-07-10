package schema

import "testing"

func TestJSONFormMapsTypesConstraintsAndUI(t *testing.T) {
	s := Schema{Fields: []Field{
		{Key: "host", Label: "服务器", Type: String, Required: true, MaxLen: 253},
		{Key: "port", Label: "端口", Type: Int, Min: 1, Max: 65535},
		{Key: "key", Label: "密钥", Type: String, Secret: true},
		{Key: "mode", Label: "模式", Type: Enum, Options: []Option{
			{Value: "a", Label: "甲", Description: "甲说明"},
			{Value: "b", Label: "乙"},
		}},
	}}
	js, ui := s.JSONForm()

	props, _ := js["properties"].(map[string]any)
	host, _ := props["host"].(map[string]any)
	if host["type"] != "string" || host["maxLength"] != 253 {
		t.Fatalf("host schema wrong: %v", host)
	}
	port, _ := props["port"].(map[string]any)
	if port["type"] != "integer" || port["minimum"] != 1 || port["maximum"] != 65535 {
		t.Fatalf("port schema wrong: %v", port)
	}
	required, _ := js["required"].([]any)
	if len(required) != 1 || required[0] != "host" {
		t.Fatalf("required wrong: %v", required)
	}
	mode, _ := props["mode"].(map[string]any)
	oneOf, _ := mode["oneOf"].([]any)
	first, _ := oneOf[0].(map[string]any)
	if len(oneOf) != 2 || first["const"] != "a" || first["title"] != "甲" {
		t.Fatalf("mode oneOf wrong: %v", oneOf)
	}

	if _, ok := ui["ui:order"]; !ok {
		t.Fatal("ui:order missing")
	}
	keyUI, _ := ui["key"].(map[string]any)
	if keyUI["ui:widget"] != "secret" {
		t.Fatalf("secret widget wrong: %v", keyUI)
	}
	modeUI, _ := ui["mode"].(map[string]any)
	modeOpts, _ := modeUI["ui:options"].(map[string]any)
	descs, _ := modeOpts["descriptions"].(map[string]any)
	if descs["a"] != "甲说明" {
		t.Fatalf("descriptions wrong: %v", descs)
	}
}

func TestJSONFormConditionalBecomesIfThen(t *testing.T) {
	s := Schema{Fields: []Field{
		{Key: "mode", Label: "模式", Type: Enum, Options: []Option{{Value: "x"}, {Value: "y"}}},
		{
			Key:      "extra",
			Label:    "附加",
			Type:     String,
			Required: true,
			ShowWhen: &ShowWhen{Field: "mode", Values: []string{"y"}},
		},
	}}
	js, _ := s.JSONForm()

	props, _ := js["properties"].(map[string]any)
	if _, ok := props["extra"]; ok {
		t.Fatal("conditional field must not be an unconditional property")
	}
	allOf, _ := js["allOf"].([]any)
	if len(allOf) != 1 {
		t.Fatalf("expected 1 allOf clause, got %d", len(allOf))
	}
	clause, _ := allOf[0].(map[string]any)
	ifClause, _ := clause["if"].(map[string]any)
	ifProps, _ := ifClause["properties"].(map[string]any)
	modeIf, _ := ifProps["mode"].(map[string]any)
	enum, _ := modeIf["enum"].([]any)
	if len(enum) != 1 || enum[0] != "y" {
		t.Fatalf("if enum wrong: %v", enum)
	}
	then, _ := clause["then"].(map[string]any)
	thenProps, _ := then["properties"].(map[string]any)
	if _, ok := thenProps["extra"]; !ok {
		t.Fatal("conditional field must be in then.properties")
	}
}
