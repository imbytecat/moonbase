package sms

import "testing"

// TestSchemaDrivesEngine proves the driver-declared aliyun schema works with
// base's generic engine end to end: secrets masked, kept on empty update,
// required + maxlen enforced — with zero masking code in this driver.
func TestSchemaDrivesEngine(t *testing.T) {
	s := Schemas()["aliyun"]

	full := map[string]any{
		"accessKeyId":     "AK123",
		"accessKeySecret": "sekret",
		"signName":        "MyApp",
		"templateCode":    "SMS_1",
	}
	if !s.Usable(full) {
		t.Fatal("fully configured profile should be usable")
	}
	if err := s.Validate(full); err != nil {
		t.Fatalf("valid config should pass: %v", err)
	}

	masked := s.Mask(full)
	if masked["accessKeySecret"] != "" {
		t.Fatal("secret must be blanked on read")
	}
	if masked["accessKeySecret_set"] != true {
		t.Fatal("stored secret must flag set=true")
	}
	if masked["accessKeyId"] != "AK123" {
		t.Fatal("non-secret must pass through")
	}

	// An update that leaves the secret blank must keep the stored one.
	merged := s.Merge(map[string]any{"accessKeyId": "AK123", "accessKeySecret": ""}, full)
	if merged["accessKeySecret"] != "sekret" {
		t.Fatalf("empty secret must keep stored, got %v", merged["accessKeySecret"])
	}

	if s.Usable(map[string]any{"accessKeyId": "AK123"}) {
		t.Fatal("missing required fields should be unusable")
	}
	over := map[string]any{"accessKeyId": string(make([]byte, 129)), "accessKeySecret": "x", "signName": "n", "templateCode": "t"}
	if err := s.Validate(over); err == nil {
		t.Fatal("over-maxlen accessKeyId should fail")
	}
}
