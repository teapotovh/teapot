package bottin

import (
	"github.com/teapotovh/teapot/lib/observability"
)

// ReadinessChecks implements observability.ReadinessChecks.
func (server *Bottin) ReadinessChecks() map[string]observability.Check {
	return server.store.ReadinessChecks()
}
