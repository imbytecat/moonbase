// Package config loads application configuration via viper: sane built-in
// defaults overridden by MOONBASE_* environment variables. No config file, no
// .env — the full knob list lives in the README deploy section.
package config

import (
	"fmt"
	"reflect"
	"strconv"
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
	Host string `mapstructure:"host" default:"0.0.0.0"`
	Port int    `mapstructure:"port" default:"8080"`
	// PublicURL is the externally reachable origin, used in emailed links.
	PublicURL string `mapstructure:"public_url" default:"http://localhost:5173"`
}

type DatabaseConfig struct {
	URL string `mapstructure:"url" default:"postgres://postgres:postgres@localhost:5432/app?sslmode=disable"`
}

type CORSConfig struct {
	AllowedOrigins []string `mapstructure:"allowed_origins" default:"http://localhost:5173"`
}

// AuthConfig controls sessions and the initial admin seed. The admin account
// is only created when the users table is empty; change the password after
// first login (defaults exist so `moon run :dev` works with zero setup, and
// they're overridable via MOONBASE_AUTH_ADMIN_USERNAME / _ADMIN_PASSWORD).
// No admin email on purpose: the identifier model makes email optional, and
// a real one is bound later via the profile page (code-verified).
type AuthConfig struct {
	SessionTTLHours         int    `mapstructure:"session_ttl_hours" default:"168"`
	SessionMaxLifetimeHours int    `mapstructure:"session_max_lifetime_hours" default:"720"`
	SecureCookie            bool   `mapstructure:"secure_cookie" default:"false"`
	AdminUsername           string `mapstructure:"admin_username" default:"admin"`
	AdminPassword           string `mapstructure:"admin_password" default:"admin123"`
}

func (a AuthConfig) SessionTTL() time.Duration {
	return time.Duration(a.SessionTTLHours) * time.Hour
}

// AuditConfig controls audit-trail retention: rows older than RetentionDays
// are deleted by the hourly janitor. 0 disables cleanup (keep forever).
type AuditConfig struct {
	RetentionDays int `mapstructure:"retention_days" default:"180"`
}

func (a AuditConfig) Retention() time.Duration {
	return time.Duration(a.RetentionDays) * 24 * time.Hour
}

// MetricsConfig controls the Prometheus endpoint. When Enabled, the server
// mounts /metrics (outside the /api authn chain, so scrapers need no session)
// and records per-RPC counters/histograms. Restrict scrape access at the
// network layer — the endpoint itself is unauthenticated by design.
type MetricsConfig struct {
	Enabled bool `mapstructure:"enabled" default:"true"`
}

// OtelConfig controls OpenTelemetry tracing. It is DORMANT by default: with an
// empty TraceEndpoint no exporter, provider, or span processing is created, so
// the RPC interceptor's spans are cheap no-ops. Set TraceEndpoint to an OTLP
// gRPC collector (host:port) to turn tracing on; SampleRatio (0..1) then caps
// the fraction of traces recorded.
type OtelConfig struct {
	// TraceEndpoint is the OTLP/gRPC collector address, e.g. "localhost:4317".
	// Empty disables tracing entirely.
	TraceEndpoint string `mapstructure:"trace_endpoint" default:""`
	ServiceName   string `mapstructure:"service_name" default:"moonbase"`
	// Insecure sends OTLP over plaintext (no TLS) — for a local collector.
	Insecure bool `mapstructure:"insecure" default:"false"`
	// SampleRatio is the head-based sampling probability in [0,1]. 1 records
	// every trace; 0.1 records 10%. Parent-sampled traces are always kept.
	SampleRatio float64 `mapstructure:"sample_ratio" default:"1.0"`
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
	Level string `mapstructure:"level" default:"info"`
	// Format forces the console handler: auto (TTY→pretty, else JSON), pretty, json.
	Format string `mapstructure:"format" default:"auto"`
	// File enables the rotating file sink when set to a path, e.g. /var/log/moonbase/server.log.
	File string `mapstructure:"file" default:""`
	// FileMaxSizeMB rotates the file when it exceeds this size in megabytes.
	FileMaxSizeMB int `mapstructure:"file_max_size_mb" default:"100"`
	// FileMaxBackups caps retained rotated files (0 = keep all).
	FileMaxBackups int `mapstructure:"file_max_backups" default:"10"`
	// FileMaxAgeDays deletes rotated files older than this many days (0 = keep forever).
	FileMaxAgeDays int `mapstructure:"file_max_age_days" default:"30"`
	// FileRotateAt additionally rotates on a clock schedule: "", "midnight", "hourly".
	FileRotateAt string `mapstructure:"file_rotate_at" default:"midnight"`
	// FileCompress gzips rotated files.
	FileCompress bool `mapstructure:"file_compress" default:"true"`
	// SQL logs every SQL statement with args and duration at debug level.
	SQL bool `mapstructure:"sql" default:"false"`
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

	bindDefaults(v, reflect.TypeOf(Config{}), "")

	v.SetEnvPrefix("MOONBASE")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}
	return &cfg, nil
}

// bindDefaults registers each leaf field's built-in default under the dotted
// viper key derived from its mapstructure tags. Registering the key is also
// what lets AutomaticEnv pick up the field's MOONBASE_* override — viper only
// reads env for keys it already knows — so the `default:` tag is the single
// source for both the default value and env binding. Adding a field with a
// default tag is all it takes; there is no parallel SetDefault list to sync.
func bindDefaults(v *viper.Viper, t reflect.Type, prefix string) {
	for i := range t.NumField() {
		field := t.Field(i)
		key := field.Tag.Get("mapstructure")
		if key == "" || key == "-" {
			continue
		}
		if prefix != "" {
			key = prefix + "." + key
		}
		if field.Type.Kind() == reflect.Struct {
			bindDefaults(v, field.Type, key)
			continue
		}
		if def, ok := field.Tag.Lookup("default"); ok {
			v.SetDefault(key, defaultValue(field.Type, def))
		}
	}
}

// defaultValue parses a `default:` tag literal into the field's Go type so the
// registered default has the same shape a decoded value would. A comma-joined
// literal on a []string field becomes the slice (mirroring viper's env split).
func defaultValue(t reflect.Type, raw string) any {
	switch t.Kind() {
	case reflect.Bool:
		b, _ := strconv.ParseBool(raw)
		return b
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n, _ := strconv.ParseInt(raw, 10, 64)
		return n
	case reflect.Float32, reflect.Float64:
		f, _ := strconv.ParseFloat(raw, 64)
		return f
	case reflect.Slice:
		if t.Elem().Kind() == reflect.String {
			if raw == "" {
				return []string{}
			}
			return strings.Split(raw, ",")
		}
	}
	return raw
}
