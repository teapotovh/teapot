package httpsrv

import (
	"context"
	"errors"

	"github.com/teapotovh/teapot/lib/observability"
)

var (
	ErrNotStartedYet = errors.New("not started yet")
	ErrNotRunning    = errors.New("server is not running")
)

func (h *HTTPSrv) hasServerStarted(ctx context.Context) error {
	if !h.running.Load() {
		return ErrNotStartedYet
	}

	return nil
}

// ReadinessChecks implements observability.ReadinessChecks.
func (h *HTTPSrv) ReadinessChecks() map[string]observability.Check {
	return map[string]observability.Check{
		"httpsrv/started": observability.CheckFunc(h.hasServerStarted),
	}
}

func (h *HTTPSrv) isServerRunning(ctx context.Context) (err error) {
	if !h.running.Load() {
		return ErrNotRunning
	}

	return nil
}

// LivenessChecks implements observability.LivenessChecks.
func (h *HTTPSrv) LivenessChecks() map[string]observability.Check {
	return map[string]observability.Check{
		"httpsrv/running": observability.CheckFunc(h.isServerRunning),
	}
}
