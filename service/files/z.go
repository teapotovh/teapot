package files

import (
	"maps"

	"github.com/teapotovh/teapot/lib/observability"
)

// ReadinessChecks implements observability.ReadinessChecks.
func (f *Files) ReadinessChecks() map[string]observability.Check {
	checks := f.store.ReadinessChecks()
	maps.Copy(checks, f.ldapFactory.ReadinessChecks())

	return checks
}

// LivenessChecks implements observability.LivenessChecks.
func (f *Files) LivenessChecks() map[string]observability.Check {
	checks := f.store.LivenessChecks()
	maps.Copy(checks, f.ldapFactory.LivenessChecks())

	return checks
}
