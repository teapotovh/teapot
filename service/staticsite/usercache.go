package staticsite

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/teapotovh/teapot/lib/ldap"
	"github.com/teapotovh/teapot/lib/run"
)

// ErrInvalidInterval reports a refresh interval that is not positive.
var ErrInvalidInterval = errors.New("refresh interval must be positive")

// UserCacheConfig configures the user cache.
type UserCacheConfig struct {
	// Interval is the time between two user list refreshes.
	Interval time.Duration
}

// UserLister lists all users from an external directory.
type UserLister interface {
	// ListUsers returns all users.
	ListUsers(ctx context.Context) ([]*ldap.User, error)
}

type ldapUserLister struct {
	factory *ldap.Factory
}

// ListUsers implements UserLister.
func (l ldapUserLister) ListUsers(ctx context.Context) ([]*ldap.User, error) {
	client, err := l.factory.NewClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("could not create LDAP client: %w", err)
	}

	defer client.Close()

	return client.Users(ctx)
}

// UserCache keeps the username to uid mapping in memory.
type UserCache struct {
	logger *slog.Logger

	ldapFactory *ldap.Factory
	lister      UserLister
	interval    time.Duration

	users atomic.Pointer[map[string]int64]
}

// NewUserCache creates a user cache.
func NewUserCache(config UserCacheConfig, ldapFactory *ldap.Factory, logger *slog.Logger) (*UserCache, error) {
	return newUserCache(config, ldapFactory, ldapUserLister{factory: ldapFactory}, logger)
}

func newUserCache(
	config UserCacheConfig,
	ldapFactory *ldap.Factory,
	lister UserLister,
	logger *slog.Logger,
) (*UserCache, error) {
	if config.Interval <= 0 {
		return nil, ErrInvalidInterval
	}

	return &UserCache{
		logger: logger,

		ldapFactory: ldapFactory,
		lister:      lister,
		interval:    config.Interval,
	}, nil
}

// UID returns the uid of a username and reports whether the user is cached.
func (c *UserCache) UID(username string) (int64, bool) {
	users := c.users.Load()
	if users == nil {
		return 0, false
	}

	uid, ok := (*users)[username]

	return uid, ok
}

// Run implements run.Runnable.
func (c *UserCache) Run(step run.Step) error {
	step.Add("ldap", c.ldapFactory)
	step.Wait()

	ctx := step.Context()

	if err := c.refresh(ctx); err != nil {
		return fmt.Errorf("could not refresh user cache: %w", err)
	}

	step.Notify()

	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := c.refresh(ctx); err != nil {
				c.logger.WarnContext(ctx, "could not refresh user cache", "err", err)
			}
		}
	}
}

func (c *UserCache) refresh(ctx context.Context) error {
	list, err := c.lister.ListUsers(ctx)
	if err != nil {
		return fmt.Errorf("could not list users: %w", err)
	}

	users := make(map[string]int64, len(list))

	for _, user := range list {
		users[user.Username] = int64(user.UID)
	}

	c.users.Store(&users)

	return nil
}

// Ensure UserCache implements run.Runnable.
var _ run.Runnable = (*UserCache)(nil)
