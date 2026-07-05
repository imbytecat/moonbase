package config

import (
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/spf13/viper"
)

// clearMoonbaseEnv neutralizes every inherited MOONBASE_* var so built-in
// defaults surface deterministically. Load() never enables AllowEmptyEnv, so
// viper treats an empty env var as absent — setting each to "" is the same as
// unsetting it, and t.Setenv restores the caller's environment afterward.
func clearMoonbaseEnv(t *testing.T) {
	t.Helper()
	for _, kv := range os.Environ() {
		if key, _, _ := strings.Cut(kv, "="); strings.HasPrefix(key, "MOONBASE_") {
			t.Setenv(key, "")
		}
	}
}

// TestLoadDefaults pins the built-in default of every field to a known-good
// literal (the README deploy table), read through the Load() seam. It is the
// behavioral spec the single-source tag mechanism must reproduce.
func TestLoadDefaults(t *testing.T) {
	clearMoonbaseEnv(t)

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}

	checks := []struct {
		name string
		got  any
		want any
	}{
		{"server.host", cfg.Server.Host, "0.0.0.0"},
		{"server.port", cfg.Server.Port, 8080},
		{"server.public_url", cfg.Server.PublicURL, "http://localhost:5173"},
		{"database.url", cfg.Database.URL, "postgres://postgres:postgres@localhost:5432/app?sslmode=disable"},
		{"auth.session_ttl_hours", cfg.Auth.SessionTTLHours, 168},
		{"auth.session_max_lifetime_hours", cfg.Auth.SessionMaxLifetimeHours, 720},
		{"auth.secure_cookie", cfg.Auth.SecureCookie, false},
		{"auth.admin_username", cfg.Auth.AdminUsername, "admin"},
		{"auth.admin_password", cfg.Auth.AdminPassword, "admin123"},
		{"audit.retention_days", cfg.Audit.RetentionDays, 180},
		{"log.level", cfg.Log.Level, "info"},
		{"log.format", cfg.Log.Format, "auto"},
		{"log.file", cfg.Log.File, ""},
		{"log.file_max_size_mb", cfg.Log.FileMaxSizeMB, 100},
		{"log.file_max_backups", cfg.Log.FileMaxBackups, 10},
		{"log.file_max_age_days", cfg.Log.FileMaxAgeDays, 30},
		{"log.file_rotate_at", cfg.Log.FileRotateAt, "midnight"},
		{"log.file_compress", cfg.Log.FileCompress, true},
		{"log.sql", cfg.Log.SQL, false},
		{"metrics.enabled", cfg.Metrics.Enabled, true},
		{"otel.trace_endpoint", cfg.Otel.TraceEndpoint, ""},
		{"otel.service_name", cfg.Otel.ServiceName, "moonbase"},
		{"otel.insecure", cfg.Otel.Insecure, false},
		{"otel.sample_ratio", cfg.Otel.SampleRatio, 1.0},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %v, want %v (default)", c.name, c.got, c.want)
		}
	}
	if len(cfg.CORS.AllowedOrigins) != 1 || cfg.CORS.AllowedOrigins[0] != "http://localhost:5173" {
		t.Errorf("cors.allowed_origins = %v, want [http://localhost:5173] (default)", cfg.CORS.AllowedOrigins)
	}
}

// TestLoadEnvOverrides is the behavioral backstop for the single-source tag
// mechanism: every field must still be overridable from its MOONBASE_* var,
// including the comma-split []string (CORS) and the int/bool/float parses.
func TestLoadEnvOverrides(t *testing.T) {
	t.Setenv("MOONBASE_SERVER_HOST", "127.0.0.1")
	t.Setenv("MOONBASE_SERVER_PORT", "9999")
	t.Setenv("MOONBASE_SERVER_PUBLIC_URL", "https://example.com")
	t.Setenv("MOONBASE_DATABASE_URL", "postgres://x/y")
	t.Setenv("MOONBASE_CORS_ALLOWED_ORIGINS", "https://a.example,https://b.example")
	t.Setenv("MOONBASE_AUTH_SESSION_TTL_HOURS", "1")
	t.Setenv("MOONBASE_AUTH_SESSION_MAX_LIFETIME_HOURS", "2")
	t.Setenv("MOONBASE_AUTH_SECURE_COOKIE", "true")
	t.Setenv("MOONBASE_AUTH_ADMIN_USERNAME", "root")
	t.Setenv("MOONBASE_AUTH_ADMIN_PASSWORD", "s3cret")
	t.Setenv("MOONBASE_AUDIT_RETENTION_DAYS", "30")
	t.Setenv("MOONBASE_LOG_LEVEL", "debug")
	t.Setenv("MOONBASE_LOG_FORMAT", "json")
	t.Setenv("MOONBASE_LOG_FILE", "/tmp/moonbase.log")
	t.Setenv("MOONBASE_LOG_FILE_MAX_SIZE_MB", "50")
	t.Setenv("MOONBASE_LOG_FILE_MAX_BACKUPS", "5")
	t.Setenv("MOONBASE_LOG_FILE_MAX_AGE_DAYS", "7")
	t.Setenv("MOONBASE_LOG_FILE_ROTATE_AT", "hourly")
	t.Setenv("MOONBASE_LOG_FILE_COMPRESS", "false")
	t.Setenv("MOONBASE_LOG_SQL", "true")
	t.Setenv("MOONBASE_METRICS_ENABLED", "false")
	t.Setenv("MOONBASE_OTEL_TRACE_ENDPOINT", "localhost:4317")
	t.Setenv("MOONBASE_OTEL_SERVICE_NAME", "moonbase-test")
	t.Setenv("MOONBASE_OTEL_INSECURE", "true")
	t.Setenv("MOONBASE_OTEL_SAMPLE_RATIO", "0.25")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}

	checks := []struct {
		name string
		got  any
		want any
	}{
		{"server.host", cfg.Server.Host, "127.0.0.1"},
		{"server.port", cfg.Server.Port, 9999},
		{"server.public_url", cfg.Server.PublicURL, "https://example.com"},
		{"database.url", cfg.Database.URL, "postgres://x/y"},
		{"auth.session_ttl_hours", cfg.Auth.SessionTTLHours, 1},
		{"auth.session_max_lifetime_hours", cfg.Auth.SessionMaxLifetimeHours, 2},
		{"auth.secure_cookie", cfg.Auth.SecureCookie, true},
		{"auth.admin_username", cfg.Auth.AdminUsername, "root"},
		{"auth.admin_password", cfg.Auth.AdminPassword, "s3cret"},
		{"audit.retention_days", cfg.Audit.RetentionDays, 30},
		{"log.level", cfg.Log.Level, "debug"},
		{"log.format", cfg.Log.Format, "json"},
		{"log.file", cfg.Log.File, "/tmp/moonbase.log"},
		{"log.file_max_size_mb", cfg.Log.FileMaxSizeMB, 50},
		{"log.file_max_backups", cfg.Log.FileMaxBackups, 5},
		{"log.file_max_age_days", cfg.Log.FileMaxAgeDays, 7},
		{"log.file_rotate_at", cfg.Log.FileRotateAt, "hourly"},
		{"log.file_compress", cfg.Log.FileCompress, false},
		{"log.sql", cfg.Log.SQL, true},
		{"metrics.enabled", cfg.Metrics.Enabled, false},
		{"otel.trace_endpoint", cfg.Otel.TraceEndpoint, "localhost:4317"},
		{"otel.service_name", cfg.Otel.ServiceName, "moonbase-test"},
		{"otel.insecure", cfg.Otel.Insecure, true},
		{"otel.sample_ratio", cfg.Otel.SampleRatio, 0.25},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %v, want %v (missing default tag on the field?)", c.name, c.got, c.want)
		}
	}
	if len(cfg.CORS.AllowedOrigins) != 2 || cfg.CORS.AllowedOrigins[0] != "https://a.example" {
		t.Errorf("cors.allowed_origins = %v, want two origins (missing default tag on the field?)", cfg.CORS.AllowedOrigins)
	}
}

type sampleNested struct {
	Count int    `mapstructure:"count" default:"3"`
	Label string `mapstructure:"label" default:"hi"`
}

type sample struct {
	Name    string       `mapstructure:"name" default:"anon"`
	Enabled bool         `mapstructure:"enabled" default:"true"`
	Tags    []string     `mapstructure:"tags" default:"a,b"`
	Nested  sampleNested `mapstructure:"nested"`
}

// TestBindDefaultsFromTags is the single-source guarantee (issue acceptance 2):
// a field declared with only its mapstructure+default tags gets both its
// built-in default and its env override for free — no manifest code to touch.
// It exercises the mechanism on a representative struct (scalar, bool, split
// []string, nested key) so a regression fails here rather than by silently
// dropping some real field's override.
func TestBindDefaultsFromTags(t *testing.T) {
	t.Setenv("MBTEST_NESTED_COUNT", "7")

	v := viper.New()
	bindDefaults(v, reflect.TypeOf(sample{}), "")
	v.SetEnvPrefix("MBTEST")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	var got sample
	if err := v.Unmarshal(&got); err != nil {
		t.Fatal(err)
	}

	if got.Name != "anon" {
		t.Errorf("name = %q, want default %q", got.Name, "anon")
	}
	if !got.Enabled {
		t.Errorf("enabled = %v, want default true", got.Enabled)
	}
	if !slices.Equal(got.Tags, []string{"a", "b"}) {
		t.Errorf("tags = %v, want split default [a b]", got.Tags)
	}
	if got.Nested.Label != "hi" {
		t.Errorf("nested.label = %q, want default %q", got.Nested.Label, "hi")
	}
	if got.Nested.Count != 7 {
		t.Errorf("nested.count = %d, want env override 7", got.Nested.Count)
	}
}
