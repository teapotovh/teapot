package files

import (
	"github.com/prometheus/client_golang/prometheus"
)

// Metrics implements observability.Metrics.
func (f *Files) Metrics() []prometheus.Collector {
	// TODO: define metrics for this module
	// The LDAP factory metrics are exported via the webdav and web
	// components, which use it for authentication.
	return []prometheus.Collector{}
}
