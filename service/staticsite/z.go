package staticsite

import (
	"github.com/teapotovh/teapot/lib/observability"
)

// ReadinessChecks implements observability.ReadinessChecks.
func (s *StaticSite) ReadinessChecks() map[string]observability.Check {
	return s.ldapFactory.ReadinessChecks()
}

// LivenessChecks implements observability.LivenessChecks.
func (s *StaticSite) LivenessChecks() map[string]observability.Check {
	return s.ldapFactory.LivenessChecks()
}
