package store

import (
	"github.com/prometheus/client_golang/prometheus"
)

type metrics struct {
	backend *prometheus.CounterVec
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
}
