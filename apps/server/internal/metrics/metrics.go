// Package metrics exposes Prometheus instrumentation for the server: a Connect
// interceptor that records per-RPC counters and latency histograms, plus
// process, Go-runtime, database-pool, and build-info collectors — all behind a
// private registry so tests stay isolated and scrapes never leak default
// globals. The /metrics endpoint and interceptor are wired in package server
// only when metrics are enabled (config.metrics.enabled).
package metrics

import (
	"context"
	"net/http"
	"time"

	"connectrpc.com/connect"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const namespace = "moonbase"

// Metrics owns the private registry and the RPC instruments. Build it once with
// New and share it: register the interceptor on the Connect handlers and mount
// Handler at /metrics.
type Metrics struct {
	registry *prometheus.Registry
	requests *prometheus.CounterVec
	duration *prometheus.HistogramVec
	inFlight prometheus.Gauge
}

// PoolStatter is the subset of *pgxpool.Pool the DB collector needs. Accepting
// the interface (not the concrete pool) keeps this package free of a pgx import
// and makes the collector trivially fakeable in tests.
type PoolStatter interface {
	Stat() PoolStat
}

// New builds the registry and registers every collector. pool may be nil (unit
// tests, or a server built without a database) — the DB gauges are simply
// omitted. It panics on duplicate registration, which can only be a programmer
// error (two identical metric definitions), never runtime input.
func New(pool PoolStatter) *Metrics {
	m := &Metrics{
		registry: prometheus.NewRegistry(),

		// Dashboard: sum by (procedure) (rate(moonbase_rpc_requests_total[5m]))
		// Dashboard: sum by (code) (rate(moonbase_rpc_requests_total[5m]))
		// SLI:      1 - (sum(rate(moonbase_rpc_requests_total{code!="ok"}[5m])) / sum(rate(moonbase_rpc_requests_total[5m])))
		// Alert:    sum(rate(moonbase_rpc_requests_total{code=~"internal|unknown|unavailable|data_loss"}[5m])) / sum(rate(moonbase_rpc_requests_total[5m])) > 0.01
		requests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "rpc",
			Name:      "requests_total",
			Help:      "Total Connect RPCs handled, by procedure and result code.",
		}, []string{"procedure", "code"}),

		// Dashboard: histogram_quantile(0.99, sum(rate(moonbase_rpc_request_duration_seconds_bucket[5m])) by (le))
		// Dashboard: histogram_quantile(0.99, sum(rate(moonbase_rpc_request_duration_seconds_bucket[5m])) by (le, procedure))
		// Alert:    histogram_quantile(0.99, sum(rate(moonbase_rpc_request_duration_seconds_bucket[5m])) by (le)) > 2
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: "rpc",
			Name:      "request_duration_seconds",
			Help:      "Connect RPC handler latency in seconds, by procedure and result code.",
			Buckets:   []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
		}, []string{"procedure", "code"}),

		// Dashboard: moonbase_rpc_in_flight_requests
		// Alert:    moonbase_rpc_in_flight_requests > 500
		inFlight: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: "rpc",
			Name:      "in_flight_requests",
			Help:      "Connect RPCs currently being handled.",
		}),
	}

	m.registry.MustRegister(
		m.requests,
		m.duration,
		m.inFlight,
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		buildInfoCollector(),
	)
	if pool != nil {
		m.registry.MustRegister(newPoolCollector(pool))
	}
	return m
}

// Handler serves the private registry in the Prometheus text exposition format.
// Mount it at /metrics outside the /api authn/authz chain so a scraper (which
// carries no session) can reach it.
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{Registry: m.registry})
}

// Interceptor records one counter increment and one latency observation per
// unary RPC, and tracks in-flight concurrency. It is placed outermost in the
// chain so it also measures rejections from the authz interceptor (a
// permission_denied is a real, countable outcome). Labels are bounded:
// procedure is the fixed compile-time RPC set, code is one of ~17 Connect
// codes — never user-derived, so cardinality stays flat.
func (m *Metrics) Interceptor() connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			procedure := req.Spec().Procedure
			start := time.Now()
			m.inFlight.Inc()
			defer m.inFlight.Dec()

			resp, err := next(ctx, req)

			code := codeString(err)
			m.requests.WithLabelValues(procedure, code).Inc()
			m.duration.WithLabelValues(procedure, code).Observe(time.Since(start).Seconds())
			return resp, err
		}
	}
}

// codeString maps an RPC outcome to a bounded label value: "ok" for success,
// otherwise the Connect code (connect.CodeOf treats a nil error as Unknown, so
// the nil case must be handled explicitly).
func codeString(err error) string {
	if err == nil {
		return "ok"
	}
	return connect.CodeOf(err).String()
}
