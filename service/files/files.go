package files

import (
	"fmt"
	"log/slog"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/teapotovh/teapot/lib/ldap"
	"github.com/teapotovh/teapot/lib/observability"
)

// Files is the service instance for teapot's file storage service.
type Files struct {
	logger *slog.Logger

	sessions    *Sessions
	ldapFactory *ldap.Factory
}

type VFS uint8

const (
	VFSOS VFS = iota
)

func (vfs VFS) String() string {
	switch vfs {
	case VFSOS:
		return "os"
	default:
		return ""
	}
}

// FilesConfig is the configuration for the Files service.
type FilesConfig struct {
	LDAP     ldap.LDAPConfig
	Mounts   []string
	Sessions SessionsConfig
}

// NewFiles returns a new Files service instance.
func NewFiles(config FilesConfig, logger *slog.Logger) (*Files, error) {
	sessions, err := NewSessions(config.Sessions, logger.With("component", "sections"))
	if err != nil {
		return nil, fmt.Errorf("error while building filesystem sources: %w", err)
	}

	ldapFactory, err := ldap.NewFactory(config.LDAP, logger.With("component", "ldap"))
	if err != nil {
		return nil, fmt.Errorf("error while building LDAP factory: %w", err)
	}

	return &Files{
		logger:      logger,
		sessions:    sessions,
		ldapFactory: ldapFactory,
	}, nil
}

func (f *Files) Sesssions() *Sessions {
	return f.sessions
}

func (f *Files) LDAPFactory() *ldap.Factory {
	return f.ldapFactory
}

// Metrics implements observability.Metrics.
func (f *Files) Metrics() []prometheus.Collector {
	// TODO: define metrics for this module
	collectors := []prometheus.Collector{}

	collectors = append(collectors, f.ldapFactory.Metrics()...)

	return collectors
}

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
