package checkblocklist

import (
	"github.com/prometheus/client_golang/prometheus"
)

type metrics struct {
	blocked *prometheus.GaugeVec
	clean   *prometheus.GaugeVec
	errors  *prometheus.GaugeVec
}

func (cbl *CheckBlockList) initMetrics() {
	cbl.metrics.blocked = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "checkblocklist_blocked",
			Help: "Number of blocklists that return a blocked status",
		},
		[]string{"ip"},
	)
	cbl.metrics.clean = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "checkblocklist_clean",
			Help: "Number of blocklists that return a clean status",
		},
		[]string{"ip"},
	)
	cbl.metrics.errors = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "checkblocklist_errors",
			Help: "Number of blocklists that return an error while querying",
		},
		[]string{"ip", "blocklist"},
	)
}

// Metrics implements observability.Metrics.
func (cbl *CheckBlockList) Metrics() []prometheus.Collector {
	// TODO: re-export kubeclient, kubecontroller metrics
	return []prometheus.Collector{
		cbl.metrics.blocked,
		cbl.metrics.clean,
		cbl.metrics.errors,
	}
}
