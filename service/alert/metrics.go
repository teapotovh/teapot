package alert

import (
	"github.com/prometheus/client_golang/prometheus"
)

type metrics struct {
	total    *prometheus.CounterVec
	duration *prometheus.HistogramVec
}

func (a *Alert) initMetrics() {
	a.metrics.total = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "alert_github_calls_total",
			Help: "Total number of GitHub API calls",
		},
		[]string{"code"},
	)

	a.metrics.duration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "alert_github_calls_duration",
			Help:    "GitHub API calls latency",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"code"},
	)
}

// Metrics implements observability.Metrics.
func (a *Alert) Metrics() []prometheus.Collector {
	return []prometheus.Collector{
		a.metrics.total,
		a.metrics.duration,
	}
}
