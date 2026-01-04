package webauth

import "github.com/prometheus/client_golang/prometheus"

// Metrics implements observability.Metrics.
func (wa *WebAuth) Metrics() []prometheus.Collector {
	// TODO: export more metrics. See #35
	return wa.auth.Metrics()
}
