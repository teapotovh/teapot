package store

import (
	"context"
	"errors"
	"fmt"
)

var (
	ErrInvalidBackend = errors.New("invalid backend")
)

// Store provides an interface implemented by all types of stores for LDAP entries.
type Store interface {
	// Lists all entries under the provided prefix DN. If exact is true, only
	// entries exactly matching this DN.
	List(ctx context.Context, prefix Prefix, exact bool) ([]Entry, error)

	// Begin starts a transaction through which the contents on the store can be modified.
	Begin(ctx context.Context) (Transaction, error)
}

// Transaction provides an interface to modify the LDAP entries in the store.
type Transaction interface {
	// Returns the context associated with this transaction
	Context() context.Context

	// Store inserts or updates an entry in the store.
	Store(entry Entry) error

	// Delete deletes the entry with the specified DN.
	Delete(dn DN) error

	// Commit permanently saves the changes made through this transaction to the store.
	Commit() error
}

type StoreConfig struct {
	Type string
	URL  string
}

func NewStore(config StoreConfig) (Store, error) {
	switch config.Type {
	case "mem":
		return NewMem(), nil
	case "psql":
		return NewPSQL(config.URL)
	default:
		return nil, fmt.Errorf("error instantiating store of type %q: %w", config.Type, ErrInvalidBackend)
	}
}
