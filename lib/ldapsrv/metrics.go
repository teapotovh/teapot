package ldapsrv

import (
	"github.com/prometheus/client_golang/prometheus"
)

type metrics struct {
	active   prometheus.Gauge
	total    prometheus.Counter
	duration *prometheus.HistogramVec
}

func (s *LDAPSrv) initMetrics() {
	s.metrics.active = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "ldapsrv_connections_active",
			Help: "Current number of active LDAP connections",
		},
	)

	s.metrics.total = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "ldapsrv_connections_total",
			Help: "Total number of LDAP connection attempts",
		},
	)

	// Operation metrics
	s.metrics.duration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "ldapsrv_operation_duration_seconds",
			Help:    "LDAP operation latency",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"operation", "status"},
	)
}

// Metrics implements observability.Metrics.
func (s *LDAPSrv) Metrics() []prometheus.Collector {
	return []prometheus.Collector{
		s.metrics.active,
		s.metrics.total,
		s.metrics.duration,
	}
}
