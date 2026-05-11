package files

import "github.com/teapotovh/teapot/lib/observability"

// ReadinessChecks implements observability.ReadinessChecks.
func (f *Files) ReadinessChecks() map[string]observability.Check {
	// TODO: define metrics for this module
	return f.ldapFactory.ReadinessChecks()
}

// LivenessChecks implements observability.LivenessChecks.
func (f *Files) LivenessChecks() map[string]observability.Check {
	// TODO: define metrics for this module
	return f.ldapFactory.LivenessChecks()
}
