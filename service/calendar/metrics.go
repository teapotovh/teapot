package calendar

import (
	"github.com/prometheus/client_golang/prometheus"
)

// Metrics implements observability.Metrics.
func (c *Calendar) Metrics() []prometheus.Collector {
	// TODO: define metrics for this module
	collectors := []prometheus.Collector{}

	collectors = append(collectors, c.httpAuth.Metrics()...)

	return collectors
}
