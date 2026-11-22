package store

import (
	"context"
	"errors"
	"maps"

	"github.com/teapotovh/teapot/lib/observability"
)

// ErrNotSynced reports that the store did not load its snapshot yet.
var ErrNotSynced = errors.New("store is not synced")

// ReadinessChecks implements observability.ReadinessChecks.
func (s *Store) ReadinessChecks() map[string]observability.Check {
	checks := map[string]observability.Check{
		"files/store/synced": observability.CheckFunc(s.isSynced),
	}

	if readiness, ok := s.db.(observability.ReadinessChecks); ok { // The memory DB does not support checks.
		maps.Copy(checks, readiness.ReadinessChecks())
	}

	if readiness, ok := s.blob.(observability.ReadinessChecks); ok { // The memory blob does not support checks.
		maps.Copy(checks, readiness.ReadinessChecks())
	}

	return checks
}

// LivenessChecks implements observability.LivenessChecks.
func (s *Store) LivenessChecks() map[string]observability.Check {
	return map[string]observability.Check{
		"files/store/synced": observability.CheckFunc(s.isSynced),
	}
}

func (s *Store) isSynced(_ context.Context) error {
	if !s.synced.Load() {
		return ErrNotSynced
	}

	return nil
}
