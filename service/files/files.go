package files

import (
	"fmt"
	"log/slog"

	"github.com/teapotovh/teapot/lib/ldap"
)

// Files is the service instance for teapot's file storage service
type Files struct {
	logger *slog.Logger

	sessions    *Sessions
	ldapFactory *ldap.Factory
}

type VFS uint8

const (
	VFS_OS VFS = iota
)

func (vfs VFS) String() string {
	switch vfs {
	case VFS_OS:
		return "os"
	default:
		return ""
	}
}

// FilesConfig is the configuration for the Files service
type FilesConfig struct {
	LDAP     ldap.LDAPConfig
	Mounts   []string
	Sessions SessionsConfig
}

// NewFiles returns a new Files service instance
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
