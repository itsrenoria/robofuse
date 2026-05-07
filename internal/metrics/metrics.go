package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// metrics.go — Prometheus metrics for operational visibility.

var (
	UnrestrictTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "robofuse_unrestrict_total",
			Help: "Total unrestrict operations.",
		},
		[]string{"status"},
	)
	CycleDuration = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "robofuse_cycle_duration_seconds",
			Help:    "Sync cycle duration.",
			Buckets: prometheus.DefBuckets,
		},
	)
	RetryQueueDepth = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "robofuse_retry_queue_depth",
			Help: "Current retry queue size.",
		},
	)
	STRMTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "robofuse_strm_total",
			Help: "Total STRM file operations.",
		},
		[]string{"action"},
	)
)

func init() {
	prometheus.MustRegister(UnrestrictTotal, CycleDuration, RetryQueueDepth, STRMTotal)
}

// Handler returns the /metrics HTTP handler for Prometheus scraping.
func Handler() http.Handler {
	return promhttp.Handler()
}
