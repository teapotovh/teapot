package grpcsrv

import (
	"context"
	"errors"

	"github.com/teapotovh/teapot/lib/observability"
)

var (
	ErrNotStartedYet = errors.New("not started yet")
	ErrNotRunning    = errors.New("server is not running")
)

func (g *GRPCSrv) hasServerStarted(ctx context.Context) error {
	if !g.running.Load() {
		return ErrNotStartedYet
	}

	return nil
}

// ReadinessChecks implements observability.ReadinessChecks.
func (g *GRPCSrv) ReadinessChecks() map[string]observability.Check {
	return map[string]observability.Check{
		"grpcsrv/started": observability.CheckFunc(g.hasServerStarted),
	}
}

func (g *GRPCSrv) isServerRunning(ctx context.Context) (err error) {
	if !g.running.Load() {
		return ErrNotRunning
	}

	return nil
}

// LivenessChecks implements observability.LivenessChecks.
func (g *GRPCSrv) LivenessChecks() map[string]observability.Check {
	return map[string]observability.Check{
		"grpcsrv/running": observability.CheckFunc(g.isServerRunning),
	}
}
