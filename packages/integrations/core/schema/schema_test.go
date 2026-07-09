package schema

import "testing"

func demoSchema() Schema {
	return Schema{Fields: []Field{
		{Key: "provider", Type: String, Required: true},
		{Key: "access_key_id", Type: String, Required: true, MaxLen: 8},
		{Key: "access_key_secret", Type: String, Secret: true, Required: true},
		{Key: "key", Type: String, Immutable: true, Pattern: "^[a-z][a-z0-9-]{1,31}$"},
		{Key: "region", Type: Enum, Options: []string{"cn", "us"}},
		{Key: "port", Type: Int, Min: 0, Max: 65535},
		{Key: "enabled", Type: Bool},
		{Key: "methods", Type: Strings, Options: []string{"native", "h5"}, Unique: true},
	}}
}

func TestMaskBlanksSecretAndFlagsStored(t *testing.T) {
	m := demoSchema().Mask(map[string]any{
		"access_key_secret": "shh",
		"access_key_id":     "abc",
	})
	if m["access_key_secret"] != "" {
		t.Fatalf("secret not blanked: %v", m["access_key_secret"])
	}
	if m["access_key_secret_set"] != true {
		t.Fatal("stored secret should flag set=true")
	}
	if m["access_key_id"] != "abc" {
		t.Fatal("non-secret must pass through")
	}
}

func TestMaskFlagsUnsetSecret(t *testing.T) {
	m := demoSchema().Mask(map[string]any{})
	if m["access_key_secret_set"] != false {
		t.Fatal("absent secret should flag set=false")
	}
}

func TestMergeKeepsStoredSecretOnEmpty(t *testing.T) {
	merged := demoSchema().Merge(
		map[string]any{"access_key_secret": "", "access_key_secret_set": true, "unknown": "drop"},
		map[string]any{"access_key_secret": "stored"},
	)
	if merged["access_key_secret"] != "stored" {
		t.Fatalf("empty secret should keep stored, got %v", merged["access_key_secret"])
	}
	if _, ok := merged["access_key_secret_set"]; ok {
		t.Fatal("_set companion must not persist")
	}
	if _, ok := merged["unknown"]; ok {
		t.Fatal("unknown fields must not persist")
	}
}

func TestMergeTakesNewSecret(t *testing.T) {
	merged := demoSchema().Merge(
		map[string]any{"access_key_secret": "new"},
		map[string]any{"access_key_secret": "stored"},
	)
	if merged["access_key_secret"] != "new" {
		t.Fatal("non-empty secret should win")
	}
}

func TestMergeForcesImmutableFromStored(t *testing.T) {
	merged := demoSchema().Merge(
		map[string]any{"key": "attacker"},
		map[string]any{"key": "original"},
	)
	if merged["key"] != "original" {
		t.Fatalf("immutable must keep stored, got %v", merged["key"])
	}
}

func TestValidateRequiredMaxLenEnum(t *testing.T) {
	s := demoSchema()
	if err := s.Validate(map[string]any{"access_key_id": "abc"}); err == nil {
		t.Fatal("missing required provider should fail")
	}
	full := map[string]any{"provider": "aliyun", "access_key_id": "abc", "access_key_secret": "x"}
	if err := s.Validate(full); err != nil {
		t.Fatalf("valid config should pass: %v", err)
	}
	if err := s.Validate(with(full, "access_key_id", "toolongvalue")); err == nil {
		t.Fatal("over-maxlen should fail")
	}
	if err := s.Validate(with(full, "region", "eu")); err == nil {
		t.Fatal("bad enum should fail")
	}
	if err := s.Validate(with(full, "region", "cn")); err != nil {
		t.Fatalf("good enum should pass: %v", err)
	}
}

func TestValidateRejectsUnknownAndReadOnlyFields(t *testing.T) {
	full := map[string]any{"provider": "aliyun", "access_key_id": "abc", "access_key_secret": "x"}
	if err := demoSchema().Validate(with(full, "unknown", "x")); err == nil {
		t.Fatal("unknown field should fail")
	}
	if err := demoSchema().Validate(with(full, "access_key_secret_set", true)); err == nil {
		t.Fatal("read-only _set field should fail")
	}
}

func TestValidateTypesPatternBoundsAndArray(t *testing.T) {
	full := map[string]any{"provider": "aliyun", "access_key_id": "abc", "access_key_secret": "x"}
	if err := demoSchema().Validate(with(full, "access_key_id", 123)); err == nil {
		t.Fatal("string field with numeric value should fail")
	}
	if err := demoSchema().Validate(with(full, "key", "Bad_Key")); err == nil {
		t.Fatal("pattern mismatch should fail")
	}
	if err := demoSchema().Validate(with(full, "port", 65536)); err == nil {
		t.Fatal("int above max should fail")
	}
	if err := demoSchema().Validate(with(full, "enabled", "true")); err == nil {
		t.Fatal("bool field with string value should fail")
	}
	if err := demoSchema().Validate(with(full, "methods", "native")); err == nil {
		t.Fatal("string array field with scalar value should fail")
	}
	if err := demoSchema().Validate(with(full, "methods", []any{"native", "bogus"})); err == nil {
		t.Fatal("string array with bad option should fail")
	}
	if err := demoSchema().Validate(with(full, "methods", []any{"native", "native"})); err == nil {
		t.Fatal("string array with duplicates should fail")
	}
	if err := demoSchema().Validate(with(full, "methods", []any{"native", "h5"})); err != nil {
		t.Fatalf("valid string array should pass: %v", err)
	}
}

func TestUsableRequiresRequiredPresent(t *testing.T) {
	s := demoSchema()
	if s.Usable(map[string]any{"provider": "aliyun"}) {
		t.Fatal("missing required should be unusable")
	}
	full := map[string]any{"provider": "aliyun", "access_key_id": "abc", "access_key_secret": "x"}
	if !s.Usable(full) {
		t.Fatal("all required present should be usable")
	}
}

// with returns a copy of base with one key overridden.
func with(base map[string]any, key string, val any) map[string]any {
	out := make(map[string]any, len(base)+1)
	for k, v := range base {
		out[k] = v
	}
	out[key] = val
	return out
}
