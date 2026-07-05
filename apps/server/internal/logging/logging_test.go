package logging

import (
	"compress/gzip"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imbytecat/moonbase/server/internal/config"
)

func TestNewConsoleOnly(t *testing.T) {
	logger, closeFn, err := New(config.LogConfig{Level: "info", Format: "json"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = closeFn() }()

	if logger.Enabled(t.Context(), slog.LevelDebug) {
		t.Error("debug should be disabled at info level")
	}
	if !logger.Enabled(t.Context(), slog.LevelInfo) {
		t.Error("info should be enabled")
	}
}

func TestParseLevel(t *testing.T) {
	cases := map[string]slog.Level{
		"debug": slog.LevelDebug,
		"INFO":  slog.LevelInfo,
		"Warn":  slog.LevelWarn,
		"error": slog.LevelError,
		"bogus": slog.LevelInfo,
		"":      slog.LevelInfo,
	}
	for in, want := range cases {
		if got := parseLevel(in); got != want {
			t.Errorf("parseLevel(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestNewFileSinkWritesJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "server.log")

	logger, closeFn, err := New(config.LogConfig{
		Level:         "debug",
		Format:        "json",
		File:          path,
		FileMaxSizeMB: 10,
	})
	if err != nil {
		t.Fatal(err)
	}

	logger.Info("hello", "key", "value")
	if err := closeFn(); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"msg":"hello"`) || !strings.Contains(string(data), `"key":"value"`) {
		t.Errorf("file sink missing structured entry, got: %s", data)
	}
}

func TestFileRotationAndCompression(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "server.log")

	logger, closeFn, err := New(config.LogConfig{
		Level:          "info",
		Format:         "json",
		File:           path,
		FileMaxSizeMB:  1,
		FileMaxBackups: 3,
		FileCompress:   true,
	})
	if err != nil {
		t.Fatal(err)
	}

	payload := strings.Repeat("x", 64*1024)
	for range 32 {
		logger.Info("fill", "data", payload)
	}
	if err := closeFn(); err != nil {
		t.Fatal(err)
	}

	compressed, err := filepath.Glob(filepath.Join(dir, "server-*.log.gz"))
	if err != nil {
		t.Fatal(err)
	}
	if len(compressed) == 0 {
		t.Fatal("expected at least one compressed rotated file")
	}

	f, err := os.Open(compressed[0])
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatalf("rotated file is not valid gzip: %v", err)
	}
	if _, err := io.ReadAll(gz); err != nil {
		t.Fatalf("decompress rotated file: %v", err)
	}
}

func TestFanoutReachesBothSinks(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "server.log")

	logger, closeFn, err := New(config.LogConfig{
		Level:         "warn",
		Format:        "json",
		File:          path,
		FileMaxSizeMB: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = closeFn() }()

	if logger.Enabled(t.Context(), slog.LevelInfo) {
		t.Error("info should be disabled at warn level across fanout")
	}
	if !logger.Enabled(t.Context(), slog.LevelWarn) {
		t.Error("warn should be enabled across fanout")
	}
}
