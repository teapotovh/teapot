package ldap

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

type metrics struct {
	dial     prometheus.Histogram
	active   prometheus.Gauge
	total    *prometheus.CounterVec
	duration *prometheus.HistogramVec
}

const namespace = "ldap"

const (
	metricsStatusSuccess   = "success"
	metricsStatusError     = "error"
	metricsOperationBind   = "bind"
	metricsOperationSearch = "search"
	metricsOperationPasswd = "passwd"
)

func (f *Factory) initMetrics() {
	f.metrics.dial = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "connection_dial_seconds",
			Help:      "LDAP connection dialing seconds",
			Buckets:   prometheus.DefBuckets,
		},
	)

	f.metrics.active = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "connections_active",
			Help:      "Current number of active LDAP connections",
		},
	)

	f.metrics.total = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "connections_total",
			Help:      "Total number of LDAP connection attempts",
		},
		[]string{"status"},
	)

	// Operation metrics
	f.metrics.duration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "operation_duration_seconds",
			Help:      "LDAP operation latency",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{"operation", "status"},
	)
}

func (m *metrics) observeDial(duration time.Duration) {
	m.dial.Observe(duration.Seconds())
}

func (m *metrics) incActive() {
	m.active.Inc()
}

func (m *metrics) decActive() {
	m.active.Dec()
}

func (m *metrics) incTotal(status string) {
	m.total.WithLabelValues(status).Add(1)
}

func (m *metrics) observeOperation(operation, status string, duration time.Duration) {
	m.duration.WithLabelValues(operation, status).Observe(duration.Seconds())
}

// Metrics implements observability.Metrics.
func (f *Factory) Metrics() []prometheus.Collector {
	return []prometheus.Collector{
		f.metrics.dial,
		f.metrics.active,
		f.metrics.total,
		f.metrics.duration,
	}
}
