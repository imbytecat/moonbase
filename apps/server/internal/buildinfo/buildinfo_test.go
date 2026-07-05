package buildinfo

import (
	"log/slog"
	"strings"
	"testing"
)

func TestGet(t *testing.T) {
	info := Get()
	if info.Version == "" {
		t.Error("Version is empty; it must always default to at least \"dev\"")
	}
	if !strings.HasPrefix(info.GoVersion, "go") {
		t.Errorf("GoVersion = %q, want a go... string", info.GoVersion)
	}
}

func TestLogValueGroupsAndTruncatesRevision(t *testing.T) {
	info := Info{
		Version:   "v1.2.3",
		GoVersion: "go1.26",
		Revision:  "abcdef1234567890deadbeef",
		Time:      "2026-01-01T00:00:00Z",
		Modified:  true,
	}
	v := info.LogValue()
	if v.Kind() != slog.KindGroup {
		t.Fatalf("LogValue kind = %v, want group", v.Kind())
	}

	got := map[string]string{}
	for _, a := range v.Group() {
		got[a.Key] = a.Value.String()
	}
	if got["version"] != "v1.2.3" {
		t.Errorf("version = %q, want v1.2.3", got["version"])
	}
	if got["revision"] != "abcdef123456" {
		t.Errorf("revision = %q, want 12-char truncation", got["revision"])
	}
	if got["dirty"] != "true" {
		t.Errorf("dirty = %q, want true", got["dirty"])
	}
}

func TestLogValueOmitsEmptyOptionalFields(t *testing.T) {
	v := Info{Version: "dev", GoVersion: "go1.26"}.LogValue()
	for _, a := range v.Group() {
		if a.Key == "revision" || a.Key == "time" || a.Key == "dirty" {
			t.Errorf("unset optional field %q should be omitted", a.Key)
		}
	}
}
