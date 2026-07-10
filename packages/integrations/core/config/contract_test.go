package config

import "testing"

type smtpTestConfig struct {
	Host string `json:"host" jsonschema:"required,title=服务器地址,minLength=1,maxLength=253"`
	Port int    `json:"port" jsonschema:"required,title=端口,minimum=1,maximum=65535"`
}

func TestContractValidatesAndDecodesTypedConfig(t *testing.T) {
	contract := MustContract[smtpTestConfig](Policy{})

	schema := contract.JSONSchema()
	if schema["type"] != "object" || schema["additionalProperties"] != false {
		t.Fatalf("schema = %v, want a closed object", schema)
	}

	canonical, err := contract.Create(map[string]any{
		"host": "smtp.example.com",
		"port": 587.0,
	})
	if err != nil {
		t.Fatalf("Create(valid) = %v", err)
	}
	typed, err := contract.Decode(canonical)
	if err != nil {
		t.Fatalf("Decode(canonical) = %v", err)
	}
	if typed.Host != "smtp.example.com" || typed.Port != 587 {
		t.Fatalf("typed config = %+v", typed)
	}

	if _, err := contract.Create(map[string]any{
		"host": "smtp.example.com",
		"port": 587.5,
	}); err == nil {
		t.Fatal("fractional integer must fail schema validation")
	}
	if _, err := contract.Create(map[string]any{
		"host":    "smtp.example.com",
		"port":    587,
		"unknown": true,
	}); err == nil {
		t.Fatal("unknown field must fail schema validation")
	}
}

type lifecycleTestConfig struct {
	Endpoint string `json:"endpoint" jsonschema:"required,minLength=1"`
	Password string `json:"password" jsonschema:"required,minLength=1"`
	Key      string `json:"key" jsonschema:"required,minLength=1"`
}

func TestContractAppliesLifecycleAndProjectsSafeView(t *testing.T) {
	contract := MustContract[lifecycleTestConfig](Policy{
		Secrets:    []string{"/password"},
		CreateOnly: []string{"/key"},
	})

	stored, err := contract.CreateWrite(map[string]any{
		"endpoint": "https://old.example.com",
		"key":      "stable",
	}, map[string]string{"/password": "original-secret"})
	if err != nil {
		t.Fatal(err)
	}

	view, valid := contract.View(stored)
	if !valid {
		t.Fatal("stored config should be valid")
	}
	if _, ok := view.Values["password"]; ok {
		t.Fatal("secret must not appear in the read view")
	}
	if len(view.SetSecretPaths) != 1 || view.SetSecretPaths[0] != "/password" {
		t.Fatalf("set secret paths = %v", view.SetSecretPaths)
	}

	updated, err := contract.UpdateWrite(map[string]any{
		"endpoint": "https://new.example.com",
	}, nil, stored)
	if err != nil {
		t.Fatalf("Update(keep secret and key) = %v", err)
	}
	typed, err := contract.Decode(updated)
	if err != nil {
		t.Fatal(err)
	}
	if typed.Password != "original-secret" || typed.Key != "stable" {
		t.Fatalf("updated config = %+v", typed)
	}

	replaced, err := contract.UpdateWrite(map[string]any{
		"endpoint": "https://new.example.com",
		"key":      "stable",
	}, map[string]string{"/password": "replacement-secret"}, stored)
	if err != nil {
		t.Fatalf("Update(replace secret) = %v", err)
	}
	replacedTyped, _ := contract.Decode(replaced)
	if replacedTyped.Password != "replacement-secret" {
		t.Fatalf("password = %q", replacedTyped.Password)
	}

	if _, err := contract.UpdateWrite(map[string]any{
		"endpoint": "https://new.example.com",
	}, map[string]string{"/password": ""}, stored); err == nil {
		t.Fatal("empty secret replacement must fail")
	}
	if _, err := contract.UpdateWrite(map[string]any{
		"endpoint": "https://new.example.com",
		"key":      "changed",
	}, nil, stored); err == nil {
		t.Fatal("changing create-only field must fail")
	}
	if _, err := contract.CreateWrite(map[string]any{
		"endpoint": "https://new.example.com",
		"password": "must-not-be-here",
		"key":      "stable",
	}, nil); err == nil {
		t.Fatal("secret in ordinary values must fail")
	}
}
