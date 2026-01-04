package ldap

import (
	"github.com/prometheus/client_golang/prometheus"
)

type metrics struct {
	active   prometheus.Gauge
	total    *prometheus.CounterVec
	duration *prometheus.HistogramVec
}

const (
	metricsStatusSuccess   = "success"
	metricsStatusError     = "error"
	metricsOperationBind   = "bind"
	metricsOperationSearch = "search"
	metricsOperationPasswd = "passwd"
)

func (f *Factory) initMetrics() {
	f.metrics.active = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "ldap_connections_active",
			Help: "Current number of active LDAP connections",
		},
	)

	f.metrics.total = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ldap_connections_total",
			Help: "Total number of LDAP connection attempts",
		},
		[]string{"status"},
	)

	// Operation metrics
	f.metrics.duration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "ldap_operation_duration_seconds",
			Help:    "LDAP operation latency",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"operation", "status"},
	)
}

// Metrics implements observability.Metrics.
func (f *Factory) Metrics() []prometheus.Collector {
	return []prometheus.Collector{
		f.metrics.active,
		f.metrics.total,
		f.metrics.duration,
	}
}
