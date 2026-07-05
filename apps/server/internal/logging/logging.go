// Package logging builds the unified application logger: everything in the
// process — our code, stdlib log (via slog.SetDefault), pgx, goose, DBOS —
// funnels into one *slog.Logger that fans out to a console handler and an
// optional rotating, compressed JSON file sink.
package logging

import (
	"log/slog"
	"os"
	"strings"

	"github.com/agentine/sawmill"
	"github.com/lmittmann/tint"
	slogmulti "github.com/samber/slog-multi"
	"golang.org/x/term"

	"github.com/imbytecat/moonbase/server/internal/config"
)

// New builds the root logger from cfg. The returned close func flushes and
// closes the rotating file sink (nil-safe, always non-nil).
func New(cfg config.LogConfig) (*slog.Logger, func() error, error) {
	level := parseLevel(cfg.Level)
	console := newConsoleHandler(os.Stderr, cfg.Format, level)

	if cfg.File == "" {
		return slog.New(console), func() error { return nil }, nil
	}

	rotator := &sawmill.Logger{
		Filename:   cfg.File,
		MaxSize:    cfg.FileMaxSizeMB,
		MaxBackups: cfg.FileMaxBackups,
		MaxAge:     cfg.FileMaxAgeDays,
		RotateAt:   cfg.FileRotateAt,
		Compress:   cfg.FileCompress,
		LocalTime:  true,
	}
	file := slog.NewJSONHandler(rotator, &slog.HandlerOptions{Level: level})

	return slog.New(slogmulti.Fanout(console, file)), rotator.Close, nil
}

func parseLevel(s string) slog.Level {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func newConsoleHandler(w *os.File, format string, level slog.Level) slog.Handler {
	pretty := format == "pretty" || (format != "json" && term.IsTerminal(int(w.Fd())))
	if pretty {
		return tint.NewHandler(w, &tint.Options{Level: level})
	}
	return slog.NewJSONHandler(w, &slog.HandlerOptions{Level: level})
}
