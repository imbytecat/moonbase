package tracing

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/trace"

	"github.com/imbytecat/moonbase/server/internal/config"
)

func spanContext(t *testing.T) context.Context {
	t.Helper()
	traceID, err := trace.TraceIDFromHex("0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	spanID, err := trace.SpanIDFromHex("0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	sc := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: trace.FlagsSampled,
	})
	return trace.ContextWithSpanContext(context.Background(), sc)
}

func TestSlogHandlerInjectsTraceContext(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(NewSlogHandler(slog.NewJSONHandler(&buf, nil)))

	logger.InfoContext(context.Background(), "no span")
	if strings.Contains(buf.String(), "trace_id") {
		t.Errorf("trace_id logged without an active span: %s", buf.String())
	}

	buf.Reset()
	logger.InfoContext(spanContext(t), "with span")
	out := buf.String()
	for _, want := range []string{`"trace_id":"0123456789abcdef0123456789abcdef"`, `"span_id":"0123456789abcdef"`} {
		if !strings.Contains(out, want) {
			t.Errorf("log missing %s; got %s", want, out)
		}
	}
}

// TestSlogHandlerSurvivesWith pins that WithAttrs/WithGroup keep re-wrapping, so
// a derived logger (logger.With(...)) still injects trace context.
func TestSlogHandlerSurvivesWith(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(NewSlogHandler(slog.NewJSONHandler(&buf, nil))).With("component", "test")

	logger.InfoContext(spanContext(t), "derived")
	if !strings.Contains(buf.String(), "trace_id") {
		t.Errorf("derived logger dropped trace context: %s", buf.String())
	}
}

func TestSetupDisabledIsNoop(t *testing.T) {
	shutdown, err := Setup(context.Background(), config.OtelConfig{})
	if err != nil {
		t.Fatalf("disabled Setup errored: %v", err)
	}
	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("no-op shutdown errored: %v", err)
	}
}
