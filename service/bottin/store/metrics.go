package store

import (
	"github.com/prometheus/client_golang/prometheus"
)

type metrics struct {
	backend             *prometheus.CounterVec
	operationDuration   *prometheus.HistogramVec
	transactionDuration *prometheus.HistogramVec
}

const (
	statusSuccess = "success"
	statusError   = "error"

	operationList   = "list"
	operationStore  = "store"
	operationDelete = "delete"
)

func status(err error) string {
	if err != nil {
		return statusError
	}

	return statusSuccess
}

func (m *metrics) initMetrics(backend string) {
	m.backend = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "bottin_store_backend_total",
			Help: "Number of backends used for bottin storage",
		},
		[]string{"backend"},
	)
	// This is used purely to track which backend the current bottin instance is running
	m.backend.WithLabelValues(backend).Add(1)

	m.operationDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "bottin_store_operation_duration",
			Help:    "Duration of how long a store operation took",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"operation", "status"},
	)
	m.transactionDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "bottin_store_transaction_duration",
			Help:    "Duration of how long a whole store transaction took",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"operations", "status"},
	)
}
