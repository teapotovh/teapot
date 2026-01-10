package alertmanager

import (
	"github.com/prometheus/client_golang/prometheus"
)

type metrics struct {
	total *prometheus.CounterVec
}

const (
	metricsStatusSuccess = "success"
	metricsStatusFailed  = "failed"
)

func (am *AlertManager) initMetrics() {
	am.metrics.total = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "alert_alertmanager_webhook_total",
			Help: "Total number of AlertManager webhook invocations",
		},
		[]string{"status"},
	)
}

// Metrics implements observability.Metrics.
func (am *AlertManager) Metrics() []prometheus.Collector {
	return []prometheus.Collector{
		am.metrics.total,
	}
}
