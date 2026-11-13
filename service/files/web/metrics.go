package web

import (
	"github.com/prometheus/client_golang/prometheus"
)

// Metrics implements observability.Metrics.
func (web *Web) Metrics() []prometheus.Collector {
	// The LDAP factory metrics are owned and exposed by files.Files.
	return web.webAuth.Metrics()
}
