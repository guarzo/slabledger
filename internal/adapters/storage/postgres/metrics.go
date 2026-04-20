package postgres

import (
	"database/sql"

	"github.com/prometheus/client_golang/prometheus"
)

// DBStatsCollector is a prometheus.Collector that emits metrics derived from
// sql.DBStats on every scrape. Using a custom collector (vs. pre-registered
// gauges updated on a timer) means every scrape gets a consistent snapshot.
type DBStatsCollector struct {
	statsFn func() sql.DBStats

	maxOpen      *prometheus.Desc
	open         *prometheus.Desc
	inUse        *prometheus.Desc
	idle         *prometheus.Desc
	waitCount    *prometheus.Desc
	maxIdleClose *prometheus.Desc
	maxLifeClose *prometheus.Desc
}

// NewDBStatsCollector returns a collector that calls statsFn on every scrape.
// namespace is prepended to each metric name.
func NewDBStatsCollector(namespace string, statsFn func() sql.DBStats) *DBStatsCollector {
	ns := func(name string) string { return namespace + "_db_" + name }
	return &DBStatsCollector{
		statsFn:      statsFn,
		maxOpen:      prometheus.NewDesc(ns("max_open_connections"), "Maximum number of open connections to the database.", nil, nil),
		open:         prometheus.NewDesc(ns("open_connections"), "The number of established connections both in use and idle.", nil, nil),
		inUse:        prometheus.NewDesc(ns("in_use_connections"), "The number of connections currently in use.", nil, nil),
		idle:         prometheus.NewDesc(ns("idle_connections"), "The number of idle connections.", nil, nil),
		waitCount:    prometheus.NewDesc(ns("wait_count_total"), "The total number of connections waited for.", nil, nil),
		maxIdleClose: prometheus.NewDesc(ns("max_idle_closed_total"), "The total number of connections closed due to SetMaxIdleConns.", nil, nil),
		maxLifeClose: prometheus.NewDesc(ns("max_lifetime_closed_total"), "The total number of connections closed due to SetConnMaxLifetime.", nil, nil),
	}
}

// Describe implements prometheus.Collector.
func (c *DBStatsCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.maxOpen
	ch <- c.open
	ch <- c.inUse
	ch <- c.idle
	ch <- c.waitCount
	ch <- c.maxIdleClose
	ch <- c.maxLifeClose
}

// Collect implements prometheus.Collector.
func (c *DBStatsCollector) Collect(ch chan<- prometheus.Metric) {
	s := c.statsFn()
	ch <- prometheus.MustNewConstMetric(c.maxOpen, prometheus.GaugeValue, float64(s.MaxOpenConnections))
	ch <- prometheus.MustNewConstMetric(c.open, prometheus.GaugeValue, float64(s.OpenConnections))
	ch <- prometheus.MustNewConstMetric(c.inUse, prometheus.GaugeValue, float64(s.InUse))
	ch <- prometheus.MustNewConstMetric(c.idle, prometheus.GaugeValue, float64(s.Idle))
	ch <- prometheus.MustNewConstMetric(c.waitCount, prometheus.CounterValue, float64(s.WaitCount))
	ch <- prometheus.MustNewConstMetric(c.maxIdleClose, prometheus.CounterValue, float64(s.MaxIdleClosed))
	ch <- prometheus.MustNewConstMetric(c.maxLifeClose, prometheus.CounterValue, float64(s.MaxLifetimeClosed))
}
