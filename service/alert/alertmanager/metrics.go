package alertmanager

import (
	"github.com/prometheus/client_golang/prometheus"
)

type metrics struct {
	total *prometheus.CounterVec
}

func (am *AlertManager) initMetrics() {
	am.metrics.total = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "alert_alertmanager_webhook_total",
			Help: "Total number of AlertManager webhook invocations",
		},
		[]string{"code"},
	)
}

// Metrics implements observability.Metrics.
func (am *AlertManager) Metrics() []prometheus.Collector {
	return []prometheus.Collector{
		am.metrics.total,
	}
}
