package alertmanager

import (
	"github.com/prometheus/client_golang/prometheus"
)

type metrics struct {
	total  *prometheus.CounterVec
	alerts *prometheus.CounterVec
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

	am.metrics.alerts = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "alert_alertmanager_webhook_alerts",
			Help: "Number of alerts received from AlertManager grouped by status",
		},
		[]string{"status"},
	)
}

// Metrics implements observability.Metrics.
func (am *AlertManager) Metrics() []prometheus.Collector {
	return []prometheus.Collector{
		am.metrics.total,
		am.metrics.alerts,
	}
}
