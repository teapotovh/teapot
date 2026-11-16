package mail

import (
	"maps"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/teapotovh/teapot/lib/observability"
)

// Metrics implements observability.Metrics.
func (m *Mail) Metrics() []prometheus.Collector {
	return m.store.Metrics()
}

// ReadinessChecks implements observability.ReadinessChecks.
func (m *Mail) ReadinessChecks() map[string]observability.Check {
	checks := map[string]observability.Check{}

	maps.Copy(checks, m.store.ReadinessChecks())

	return checks
}

// LivenessChecks implements observability.LivenessChecks.
func (m *Mail) LivenessChecks() map[string]observability.Check {
	return map[string]observability.Check{}
}

var (
	_ observability.Metrics         = (*Mail)(nil)
	_ observability.ReadinessChecks = (*Mail)(nil)
	_ observability.LivenessChecks  = (*Mail)(nil)
)
