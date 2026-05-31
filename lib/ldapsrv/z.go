package ldapsrv

import (
	"context"
	"errors"

	"github.com/teapotovh/teapot/lib/observability"
)

var (
	ErrNotStartedYet = errors.New("not started yet")
	ErrNotRunning    = errors.New("server is not running")
)

func (s *LDAPSrv) hasServerStarted(ctx context.Context) error {
	if !s.running.Load() {
		return ErrNotStartedYet
	}

	return nil
}

// ReadinessChecks implements observability.ReadinessChecks.
func (s *LDAPSrv) ReadinessChecks() map[string]observability.Check {
	return map[string]observability.Check{
		"ldapsrv/started": observability.CheckFunc(s.hasServerStarted),
	}
}

func (s *LDAPSrv) isServerRunning(ctx context.Context) (err error) {
	if !s.running.Load() {
		return ErrNotRunning
	}

	return nil
}

// LivenessChecks implements observability.LivenessChecks.
func (s *LDAPSrv) LivenessChecks() map[string]observability.Check {
	return map[string]observability.Check{
		"ldapsrv/running": observability.CheckFunc(s.isServerRunning),
	}
}
