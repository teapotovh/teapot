package store

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

const (
	metricsNamespace = "bottin"
	metricsSubsystem = "store"

	metricsStatusSuccess = "success"
	metricsStatusError   = "error"

	metricsOperationList     = "list"
	metricsOperationGet      = "get"
	metricsOperationRange    = "range"
	metricsOperationBegin    = "begin"
	metricsOperationStore    = "store"
	metricsOperationDelete   = "delete"
	metricsOperationCommit   = "commit"
	metricsOperationRollback = "rollback"
)

type metrics struct {
	backend           *prometheus.CounterVec
	operationDuration *prometheus.HistogramVec
}

func (m *metrics) initMetrics(backend string) {
	m.backend = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: metricsNamespace,
			Subsystem: metricsSubsystem,
			Name:      "backend_total",
			Help:      "Number of bottin store backends",
		},
		[]string{"backend"},
	)

	m.operationDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: metricsNamespace,
			Subsystem: metricsSubsystem,
			Name:      "operation_duration_seconds",
			Help:      "Duration of bottin store operations in seconds",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{"operation", "status"},
	)

	// This is used purely to track which backend the current bottin instance is running
	m.backend.WithLabelValues(backend).Add(1)
}

func (m *metrics) observeOperation(operation string, start time.Time, err error) {
	status := metricsStatusSuccess
	if err != nil {
		status = metricsStatusError
	}

	m.operationDuration.WithLabelValues(operation, status).Observe(time.Since(start).Seconds())
}

// Metrics implements observability.Metrics.
func (p *PSQL) Metrics() []prometheus.Collector {
	return []prometheus.Collector{p.metrics.operationDuration, p.metrics.backend}
}

// Metrics implements observability.Metrics.
func (m *Mem) Metrics() []prometheus.Collector {
	return []prometheus.Collector{m.metrics.operationDuration, m.metrics.backend}
}
