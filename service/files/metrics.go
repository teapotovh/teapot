package files

import (
	"github.com/prometheus/client_golang/prometheus"
)

// Metrics implements observability.Metrics.
func (f *Files) Metrics() []prometheus.Collector {
	// TODO: define metrics for this module
	collectors := []prometheus.Collector{}

	collectors = append(collectors, f.ldapFactory.Metrics()...)

	return collectors
}
