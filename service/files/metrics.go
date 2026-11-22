package files

import (
	"github.com/prometheus/client_golang/prometheus"
)

// Metrics implements observability.Metrics.
func (f *Files) Metrics() []prometheus.Collector {
	return f.store.Metrics()
}
