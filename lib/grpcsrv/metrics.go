package grpcsrv

import "github.com/prometheus/client_golang/prometheus"

// Metrics implements observability.Metrics.
func (g *GRPCSrv) Metrics() []prometheus.Collector {
	return []prometheus.Collector{
		g.metrics,
	}
}
