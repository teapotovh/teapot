package store

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/teapotovh/teapot/lib/observability"
	"github.com/teapotovh/teapot/lib/run"
)

var (
	ErrInvalidBackend = errors.New("invalid backend")
)

// Store provides an interface implemented by all types of stores for LDAP entries.
type Store interface {
	run.Runnable
	observability.Metrics
	observability.ReadinessChecks

	// Ping allows pinging the store to verify that it is ready to accept requests.
	Ping(ctx context.Context) error

	// CreateCalendar saves a calendar in the store.
	CreateCalendar(ctx context.Context, calendar Calendar) error

	// ListCalendars returns a list of all calendar resources below the given path.
	ListCalendars(ctx context.Context, basePath Path) ([]Calendar, error)

	// GetCalendar fetches a single calendar from the store.
	GetCalendar(ctx context.Context, path Path) (*Calendar, error)

	// CreateCalendarObject inserts a calendar object into the store.
	CreateCalendarObject(ctx context.Context, object Object) error

	// ListCalendarObjects lists all calendar object under the given path from the store.
	ListCalendarObjects(ctx context.Context, path Path) ([]Object, error)

	// GetCalendarObject fetches a single calendar object from the store.
	GetCalendarObject(ctx context.Context, path Path) (*Object, error)

	// DeleteCalendarObject removes a calendar object from the store.
	DeleteCalendarObject(ctx context.Context, path Path) error
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
		return NewPSQL(ctx, config.URL, logger)
	default:
		return nil, fmt.Errorf("error instantiating store of type %q: %w", config.Type, ErrInvalidBackend)
	}
}
