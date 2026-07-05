// Package tracing wires optional OpenTelemetry tracing. It is dormant by
// default: with no OTLP endpoint configured, Setup installs nothing and the
// global tracer stays a no-op, so the otelconnect interceptor and NewSlogHandler
// add negligible cost. Configure config.otel.trace_endpoint to turn it on.
package tracing

import (
	"context"
	"fmt"
	"log/slog"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.41.0"
	"go.opentelemetry.io/otel/trace"

	"github.com/imbytecat/moonbase/server/internal/buildinfo"
	"github.com/imbytecat/moonbase/server/internal/config"
)

// Setup installs a global TracerProvider exporting to the configured OTLP/gRPC
// collector and returns its shutdown func. When tracing is disabled it returns
// a no-op shutdown and touches no globals.
func Setup(ctx context.Context, cfg config.OtelConfig) (func(context.Context) error, error) {
	noop := func(context.Context) error { return nil }
	if !cfg.Enabled() {
		return noop, nil
	}

	opts := []otlptracegrpc.Option{otlptracegrpc.WithEndpoint(cfg.TraceEndpoint)}
	if cfg.Insecure {
		opts = append(opts, otlptracegrpc.WithInsecure())
	}
	exporter, err := otlptracegrpc.New(ctx, opts...)
	if err != nil {
		return noop, fmt.Errorf("otlp trace exporter: %w", err)
	}

	res := resource.NewSchemaless(
		semconv.ServiceName(cfg.ServiceName),
		semconv.ServiceVersion(buildinfo.Get().Version),
	)
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(cfg.SampleRatio))),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))
	return tp.Shutdown, nil
}

// NewSlogHandler wraps h so context-carried log records gain trace_id/span_id
// when a valid span is active — the log↔trace correlation join key. It is a
// no-op when no span is present, so it is always safe to install.
func NewSlogHandler(h slog.Handler) slog.Handler {
	return traceHandler{Handler: h}
}

type traceHandler struct{ slog.Handler }

func (t traceHandler) Handle(ctx context.Context, r slog.Record) error {
	if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
		r.AddAttrs(
			slog.String("trace_id", sc.TraceID().String()),
			slog.String("span_id", sc.SpanID().String()),
		)
	}
	return t.Handler.Handle(ctx, r)
}

func (t traceHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return traceHandler{Handler: t.Handler.WithAttrs(attrs)}
}

func (t traceHandler) WithGroup(name string) slog.Handler {
	return traceHandler{Handler: t.Handler.WithGroup(name)}
}
