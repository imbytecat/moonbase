// Package config loads application configuration via viper: sane built-in
// defaults overridden by MOONBASE_* environment variables. No config file, no
// .env — the full knob list lives in the README deploy section.
package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	Server   ServerConfig   `mapstructure:"server"`
	Database DatabaseConfig `mapstructure:"database"`
	CORS     CORSConfig     `mapstructure:"cors"`
	Auth     AuthConfig     `mapstructure:"auth"`
	Audit    AuditConfig    `mapstructure:"audit"`
	Log      LogConfig      `mapstructure:"log"`
	Metrics  MetricsConfig  `mapstructure:"metrics"`
	Otel     OtelConfig     `mapstructure:"otel"`
}

type ServerConfig struct {
	Host string `mapstructure:"host"`
	Port int    `mapstructure:"port"`
	// PublicURL is the externally reachable origin, used in emailed links.
	PublicURL string `mapstructure:"public_url"`
}

type DatabaseConfig struct {
	URL string `mapstructure:"url"`
}

type CORSConfig struct {
	AllowedOrigins []string `mapstructure:"allowed_origins"`
}

// AuthConfig controls sessions and the initial admin seed. The admin account
// is only created when the users table is empty; change the password after
// first login (defaults exist so `moon run :dev` works with zero setup, and
// they're overridable via MOONBASE_AUTH_ADMIN_USERNAME / _ADMIN_PASSWORD).
// No admin email on purpose: the identifier model makes email optional, and
// a real one is bound later via the profile page (code-verified).
type AuthConfig struct {
	SessionTTLHours         int    `mapstructure:"session_ttl_hours"`
	SessionMaxLifetimeHours int    `mapstructure:"session_max_lifetime_hours"`
	SecureCookie            bool   `mapstructure:"secure_cookie"`
	AdminUsername           string `mapstructure:"admin_username"`
	AdminPassword           string `mapstructure:"admin_password"`
}

func (a AuthConfig) SessionTTL() time.Duration {
	return time.Duration(a.SessionTTLHours) * time.Hour
}

// AuditConfig controls audit-trail retention: rows older than RetentionDays
// are deleted by the hourly janitor. 0 disables cleanup (keep forever).
type AuditConfig struct {
	RetentionDays int `mapstructure:"retention_days"`
}

func (a AuditConfig) Retention() time.Duration {
	return time.Duration(a.RetentionDays) * 24 * time.Hour
}

// MetricsConfig controls the Prometheus endpoint. When Enabled, the server
// mounts /metrics (outside the /api authn chain, so scrapers need no session)
// and records per-RPC counters/histograms. Restrict scrape access at the
// network layer — the endpoint itself is unauthenticated by design.
type MetricsConfig struct {
	Enabled bool `mapstructure:"enabled"`
}

// OtelConfig controls OpenTelemetry tracing. It is DORMANT by default: with an
// empty TraceEndpoint no exporter, provider, or span processing is created, so
// the RPC interceptor's spans are cheap no-ops. Set TraceEndpoint to an OTLP
// gRPC collector (host:port) to turn tracing on; SampleRatio (0..1) then caps
// the fraction of traces recorded.
type OtelConfig struct {
	// TraceEndpoint is the OTLP/gRPC collector address, e.g. "localhost:4317".
	// Empty disables tracing entirely.
	TraceEndpoint string `mapstructure:"trace_endpoint"`
	ServiceName   string `mapstructure:"service_name"`
	// Insecure sends OTLP over plaintext (no TLS) — for a local collector.
	Insecure bool `mapstructure:"insecure"`
	// SampleRatio is the head-based sampling probability in [0,1]. 1 records
	// every trace; 0.1 records 10%. Parent-sampled traces are always kept.
	SampleRatio float64 `mapstructure:"sample_ratio"`
}

// Enabled reports whether tracing should be wired (an OTLP endpoint is set).
func (o OtelConfig) Enabled() bool {
	return o.TraceEndpoint != ""
}

// LogConfig controls the unified logger (internal/logging): a console handler
// (pretty when attached to a TTY, JSON otherwise) always on, plus an optional
// JSON file sink with size/time-based rotation and gzip compression of rotated
// segments. SQL toggles pgx statement tracing (debug-level, verbose).
type LogConfig struct {
	// Level is the minimum level for both sinks: debug, info, warn, error.
	Level string `mapstructure:"level"`
	// Format forces the console handler: auto (TTY→pretty, else JSON), pretty, json.
	Format string `mapstructure:"format"`
	// File enables the rotating file sink when set to a path, e.g. /var/log/moonbase/server.log.
	File string `mapstructure:"file"`
	// FileMaxSizeMB rotates the file when it exceeds this size in megabytes.
	FileMaxSizeMB int `mapstructure:"file_max_size_mb"`
	// FileMaxBackups caps retained rotated files (0 = keep all).
	FileMaxBackups int `mapstructure:"file_max_backups"`
	// FileMaxAgeDays deletes rotated files older than this many days (0 = keep forever).
	FileMaxAgeDays int `mapstructure:"file_max_age_days"`
	// FileRotateAt additionally rotates on a clock schedule: "", "midnight", "hourly".
	FileRotateAt string `mapstructure:"file_rotate_at"`
	// FileCompress gzips rotated files.
	FileCompress bool `mapstructure:"file_compress"`
	// SQL logs every SQL statement with args and duration at debug level.
	SQL bool `mapstructure:"sql"`
}

// SessionMaxLifetime caps sliding renewal: a session can keep extending its
// idle expiry but is force-expired this long after creation, so a stolen
// token can't stay alive forever by pinging the API.
func (a AuthConfig) SessionMaxLifetime() time.Duration {
	return time.Duration(a.SessionMaxLifetimeHours) * time.Hour
}

func (s ServerConfig) Addr() string {
	return fmt.Sprintf("%s:%d", s.Host, s.Port)
}

// Load builds configuration from defaults and MOONBASE_* environment variables
// ("_" replaces ".", e.g. MOONBASE_DATABASE_URL overrides database.url).
func Load() (*Config, error) {
	v := viper.New()

	v.SetDefault("server.host", "0.0.0.0")
	v.SetDefault("server.port", 8080)
	v.SetDefault("server.public_url", "http://localhost:5173")
	v.SetDefault("database.url", "postgres://postgres:postgres@localhost:5432/app?sslmode=disable")
	v.SetDefault("cors.allowed_origins", []string{"http://localhost:5173"})
	v.SetDefault("auth.session_ttl_hours", 168)
	v.SetDefault("auth.session_max_lifetime_hours", 720)
	// Every key needs a default: viper's AutomaticEnv+Unmarshal only reads
	// env vars for keys it already knows, so a missing default here means
	// the MOONBASE_* var is silently ignored (guarded by TestLoadEnvOverrides).
	v.SetDefault("auth.secure_cookie", false)
	v.SetDefault("auth.admin_username", "admin")
	v.SetDefault("auth.admin_password", "admin123")
	v.SetDefault("audit.retention_days", 180)
	v.SetDefault("log.level", "info")
	v.SetDefault("log.format", "auto")
	v.SetDefault("log.file", "")
	v.SetDefault("log.file_max_size_mb", 100)
	v.SetDefault("log.file_max_backups", 10)
	v.SetDefault("log.file_max_age_days", 30)
	v.SetDefault("log.file_rotate_at", "midnight")
	v.SetDefault("log.file_compress", true)
	v.SetDefault("log.sql", false)

	v.SetDefault("metrics.enabled", true)
	v.SetDefault("otel.trace_endpoint", "")
	v.SetDefault("otel.service_name", "moonbase")
	v.SetDefault("otel.insecure", false)
	v.SetDefault("otel.sample_ratio", 1.0)

	v.SetEnvPrefix("MOONBASE")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}
	return &cfg, nil
}
