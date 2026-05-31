package bottin

import (
	"github.com/prometheus/client_golang/prometheus"
)

// Metrics implements observability.Metrics.
func (server *Bottin) Metrics() []prometheus.Collector {
	// TODO: expose interesting metrics for Bottin
	collectors := []prometheus.Collector{}

	collectors = append(collectors, server.store.Metrics()...)

	return collectors
}
