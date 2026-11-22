package store

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

const (
	metricsNamespace = "files"
	metricsSubsystem = "store"

	metricsStatusSuccess = "success"
	metricsStatusError   = "error"

	metricsOperationStat   = "stat"
	metricsOperationList   = "list"
	metricsOperationOpen   = "open"
	metricsOperationPut    = "put"
	metricsOperationMkdir  = "mkdir"
	metricsOperationDelete = "delete"
	metricsOperationRename = "rename"
	metricsOperationGet    = "get"
)

type metrics struct {
	operationDuration     *prometheus.HistogramVec
	blobOperationDuration *prometheus.HistogramVec
	revision              prometheus.Gauge
}

func (m *metrics) initMetrics() {
	m.operationDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: metricsNamespace,
			Subsystem: metricsSubsystem,
			Name:      "operation_duration_seconds",
			Help:      "Duration of file store operations in seconds",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{"operation", "status"},
	)

	m.blobOperationDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: metricsNamespace,
			Subsystem: metricsSubsystem,
			Name:      "blob_operation_duration_seconds",
			Help:      "Duration of blob store operations in seconds",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{"operation", "status"},
	)

	m.revision = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: metricsNamespace,
			Subsystem: metricsSubsystem,
			Name:      "revision",
			Help:      "Current revision applied to the store",
		},
	)
}

func (m *metrics) observeOperation(operation string, start time.Time, err error) {
	status := metricsStatusSuccess
	if err != nil {
		status = metricsStatusError
	}

	m.operationDuration.WithLabelValues(operation, status).Observe(time.Since(start).Seconds())
}

func (m *metrics) observeBlobOperation(operation string, start time.Time, err error) {
	status := metricsStatusSuccess
	if err != nil {
		status = metricsStatusError
	}

	m.blobOperationDuration.WithLabelValues(operation, status).Observe(time.Since(start).Seconds())
}

func (m *metrics) setRevision(revision uint64) {
	m.revision.Set(float64(revision))
}

// Metrics implements observability.Metrics.
func (s *Store) Metrics() []prometheus.Collector {
	return []prometheus.Collector{
		s.metrics.operationDuration,
		s.metrics.blobOperationDuration,
		s.metrics.revision,
	}
}
