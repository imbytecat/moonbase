package config

import (
	"testing"
)

// Guards the viper footgun: AutomaticEnv+Unmarshal only sees keys that have
// a SetDefault, so a struct field without one silently ignores its MOONBASE_* var.
// Every field in Config must be overridable from the environment.
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
			t.Errorf("%s = %v, want %v (missing SetDefault in Load?)", c.name, c.got, c.want)
		}
	}
	if len(cfg.CORS.AllowedOrigins) != 2 || cfg.CORS.AllowedOrigins[0] != "https://a.example" {
		t.Errorf("cors.allowed_origins = %v, want two origins (missing SetDefault in Load?)", cfg.CORS.AllowedOrigins)
	}
}
