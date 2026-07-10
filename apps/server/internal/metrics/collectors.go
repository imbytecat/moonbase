package metrics

import (
	"github.com/prometheus/client_golang/prometheus"

	"github.com/imbytecat/moonbase/server/internal/buildinfo"
)

// PoolStat is a snapshot of database connection-pool saturation. It mirrors the
// gauge-worthy fields of pgxpool.Stat without importing pgx here, so the
// collector stays unit-testable with a plain fake.
type PoolStat struct {
	Acquired int32 // connections currently checked out
	Idle     int32 // connections open but unused
	Total    int32 // Acquired + Idle
	Max      int32 // pool ceiling
}

// poolCollector turns a live PoolStatter into four gauges scraped on demand
// (Collect runs per scrape, so the numbers are always current).
//
// Dashboard: moonbase_db_connections_acquired / moonbase_db_connections_max
// Alert:    moonbase_db_connections_acquired / moonbase_db_connections_max > 0.9
type poolCollector struct {
	statter  PoolStatter
	acquired *prometheus.Desc
	idle     *prometheus.Desc
	total    *prometheus.Desc
	max      *prometheus.Desc
}

func newPoolCollector(statter PoolStatter) *poolCollector {
	desc := func(name, help string) *prometheus.Desc {
		return prometheus.NewDesc(namespace+"_db_"+name, help, nil, nil)
	}
	return &poolCollector{
		statter: statter,
		acquired: desc(
			"connections_acquired",
			"Database connections currently checked out of the pool.",
		),
		idle: desc("connections_idle", "Database connections open but idle."),
		total: desc(
			"connections_total",
			"Database connections currently open (acquired + idle).",
		),
		max: desc("connections_max", "Maximum database connections the pool allows."),
	}
}

func (c *poolCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.acquired
	ch <- c.idle
	ch <- c.total
	ch <- c.max
}

func (c *poolCollector) Collect(ch chan<- prometheus.Metric) {
	s := c.statter.Stat()
	gauge := func(d *prometheus.Desc, v int32) {
		ch <- prometheus.MustNewConstMetric(d, prometheus.GaugeValue, float64(v))
	}
	gauge(c.acquired, s.Acquired)
	gauge(c.idle, s.Idle)
	gauge(c.total, s.Total)
	gauge(c.max, s.Max)
}

// buildInfoCollector exposes the binary's provenance as a value-1 gauge whose
// labels carry version/revision — the standard `_info` pattern that lets
// dashboards join runtime series against the deployed build.
//
// Dashboard: moonbase_build_info
// Query:     count by (version) (moonbase_build_info)
func buildInfoCollector() prometheus.Collector {
	b := buildinfo.Get()
	g := prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "build_info",
		Help:      "Build provenance; the value is always 1, the labels carry the metadata.",
		ConstLabels: prometheus.Labels{
			"version":    b.Version,
			"revision":   b.Revision,
			"go_version": b.GoVersion,
		},
	})
	g.Set(1)
	return g
}
