package kontakte

import (
	"github.com/prometheus/client_golang/prometheus"
)

// Metrics implements observability.Metrics.
func (k *Kontakte) Metrics() []prometheus.Collector {
	collectors := []prometheus.Collector{}

	collectors = append(collectors, k.webAuth.Metrics()...)
	collectors = append(collectors, k.ldapFactory.Metrics()...)

	return collectors
}
