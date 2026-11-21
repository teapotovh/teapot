package store

import (
	"context"

	"github.com/teapotovh/teapot/lib/pgnotify"
)

// Notifications sends and receives PostgreSQL notifications.
type Notifications interface {
	// Listen registers a notification channel.
	Listen(ctx context.Context, channel string) error
	// Next returns the next notification payload.
	Next(ctx context.Context) (string, []byte, error)
	// Notify sends a notification inside a transaction.
	Notify(ctx context.Context, tx pgnotify.Executor, channel string, payload []byte) error
	// Close closes the notification connection.
	Close() error
}

// Ensure *pgnotify.Manager implements Notifications.
var _ Notifications = (*pgnotify.Manager)(nil)
