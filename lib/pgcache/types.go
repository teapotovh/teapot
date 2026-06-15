package pgcache

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type (
	Less[T any] interface {
		Less(other T) bool
	}

	Key[T any] interface {
		Less[T]
		String() string
	}

	FromString[K Key[K]] func(str string) (*K, error)

	Object[K Key[K]] interface {
		Key() K
	}

	// List returns ALL objects of this type from the database.
	List[K Key[K], T Object[K]] func(ctx context.Context, conn *pgxpool.Pool) ([]T, error)

	// Get returns all objects matching the given keys from the database.
	Get[K Key[K], T Object[K]] func(ctx context.Context, conn *pgxpool.Pool, keys []K) ([]T, error)

	// Store stores the given objects in the database.
	Store[K Key[K], T Object[K]] func(ctx context.Context, tx pgx.Tx, objects []T) error

	// Delete deletes the given objects from the database.
	Delete[K Key[K], T Object[K]] func(ctx context.Context, tx pgx.Tx, keys []K) error
)
