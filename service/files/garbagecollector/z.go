package garbagecollector

import (
	"maps"

	"github.com/teapotovh/teapot/lib/observability"
)

// ReadinessChecks implements observability.ReadinessChecks.
func (c *GarbageCollector) ReadinessChecks() map[string]observability.Check {
	checks := map[string]observability.Check{}

	if readiness, ok := c.refs.(observability.ReadinessChecks); ok { // The fakes do not support checks.
		maps.Copy(checks, readiness.ReadinessChecks())
	}

	if readiness, ok := c.objects.(observability.ReadinessChecks); ok { // The fakes do not support checks.
		maps.Copy(checks, readiness.ReadinessChecks())
	}

	return checks
}
