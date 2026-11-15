package store

import (
	"crypto/sha256"

	"github.com/prometheus/client_golang/prometheus"
)

func digestBytes(data []byte) []byte {
	sum := sha256.Sum256(data)

	return append([]byte(nil), sum[:]...)
}

func randomUIDValidity() uint32 {
	return uint32(nowNano() | 1) //nolint:gosec
}

type metrics struct {
	backend *prometheus.CounterVec
}

func (m *metrics) initMetrics(backend string) {
	m.backend = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "mail_store_operations_total",
			Help: "Total number of mail store operations",
		},
		[]string{"backend", "operation", "status"},
	)
	m.backend.WithLabelValues(backend, "init", "success").Add(0)
}
