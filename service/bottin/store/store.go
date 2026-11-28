package store

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/teapotovh/teapot/lib/observability"
)

var (
	// ErrInvalidBackend means the configured store backend is unknown.
	ErrInvalidBackend = errors.New("invalid backend")

	// ErrNotFound means the requested entry does not exist.
	ErrNotFound = errors.New("not found")
)

// Store provides an interface implemented by all types of stores for LDAP entries.
type Store interface {
	observability.Metrics
	observability.ReadinessChecks

	// Ping allows pinging the store to verify that it is ready to accept requests
	Ping(ctx context.Context) error

	// Get returns the entry with the provided prefix.
	Get(ctx context.Context, prefix Prefix) (Entry, error)

	// List returns the entry and all its descendants under the provided prefix.
	List(ctx context.Context, prefix Prefix) ([]Entry, error)

	// Begin starts a transaction through which the contents on the store can be modified.
	Begin(ctx context.Context) (Transaction, error)
}

// Transaction provides an interface to modify the LDAP entries in the store.
type Transaction interface {
	// Store inserts or updates an entry in the store.
	Store(ctx context.Context, entry Entry) error

	// Delete deletes the entry with the specified DN.
	Delete(ctx context.Context, dn DN) error

	// Commit permanently saves the changes made through this transaction to the store.
	Commit(ctx context.Context) error

	// Rollback discards the changes made through this transaction.
	Rollback(ctx context.Context) error
}

type StoreConfig struct {
	Timeout time.Duration
	Type    string
	URL     string
}

func NewStore(config StoreConfig, logger *slog.Logger) (Store, error) {
	ctx, cancel := context.WithTimeout(context.Background(), config.Timeout)
	defer cancel()

	switch config.Type {
	case "mem":
		return NewMem(), nil
	case "psql":
		return NewPSQL(ctx, config.URL, logger.With("store", "psql"))
	default:
		return nil, fmt.Errorf("error instantiating store of type %q: %w", config.Type, ErrInvalidBackend)
	}
}
