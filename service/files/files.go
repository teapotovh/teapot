package files

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/teapotovh/teapot/lib/ldap"
	"github.com/teapotovh/teapot/service/files/store"
)

// Files is the service instance for teapot's file storage service.
type Files struct {
	store       *store.Store
	ldapFactory *ldap.Factory
}

// FilesConfig is the configuration for the Files service.
type FilesConfig struct {
	Store store.Config
	LDAP  ldap.LDAPConfig
}

// NewFiles returns a new Files service instance.
func NewFiles(ctx context.Context, config FilesConfig, logger *slog.Logger) (*Files, error) {
	storage, err := store.NewPSQL(ctx, config.Store, logger.With("component", "store"))
	if err != nil {
		return nil, err
	}

	ldapFactory, err := ldap.NewFactory(config.LDAP, logger.With("component", "ldap"))
	if err != nil {
		return nil, fmt.Errorf("error while building LDAP factory: %w", err)
	}

	return &Files{
		store:       storage,
		ldapFactory: ldapFactory,
	}, nil
}

// Store returns the file store.
func (f *Files) Store() *store.Store {
	return f.store
}

// Session returns a per-request view of a user and their files.
func (f *Files) Session(ctx context.Context, username string) (*Session, error) {
	client, err := f.ldapFactory.NewClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("error while creating ldap client: %w", err)
	}

	defer client.Close()

	user, err := client.User(ctx, username)
	if err != nil {
		return nil, fmt.Errorf("error while looking up user %q: %w", username, err)
	}

	return &Session{
		user: user,
		fs:   f.store.For(int64(user.UID)),
	}, nil
}

// Session is a per-request view of a user and their files.
type Session struct {
	user *ldap.User
	fs   *store.FS
}

// User returns the logged-in user.
func (s *Session) User() *ldap.User {
	return s.user
}

// FS returns the file system view for the user.
func (s *Session) FS() *store.FS {
	return s.fs
}

// LDAPFactory returns the LDAP factory.
func (f *Files) LDAPFactory() *ldap.Factory {
	return f.ldapFactory
}

// MaxSize is the maximum upload size in bytes.
const MaxSize = 64 << 20
