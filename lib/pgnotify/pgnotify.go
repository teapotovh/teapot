package pgnotify

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/teapotovh/teapot/lib/run"
)

var (
	// ErrClosing is returned when the notification connection is closing.
	ErrClosing = errors.New("notification connection is closing")
)

const (
	// KeepaliveCheckTime is the time between keepalive checks.
	KeepaliveCheckTime = 10 * time.Minute

	// PingTimeout is the timeout for a keepalive ping.
	PingTimeout = 5 * time.Second
)

// Connection is a dedicated PostgreSQL LISTEN connection.
type Connection struct {
	logger *slog.Logger

	listenConn   *pgx.Conn
	listenCancel context.CancelFunc
	channel      string
	listening    atomic.Bool
}

// NewConnection creates a notification connection.
func NewConnection(config *pgx.ConnConfig, channel string, logger *slog.Logger) (*Connection, error) {
	listenConn, err := pgx.ConnectConfig(context.Background(), config)
	if err != nil {
		return nil, fmt.Errorf("error while opening the notification connection: %w", err)
	}

	return &Connection{
		logger:     logger,
		listenConn: listenConn,
		channel:    channel,
	}, nil
}

// IsListening reports whether the connection is listening.
func (c *Connection) IsListening() bool {
	return c.listening.Load()
}

// Listen starts listening on the configured channel.
func (c *Connection) Listen(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)

	if _, err := c.listenConn.Exec(ctx, "LISTEN "+c.channel); err != nil {
		defer cancel()

		return fmt.Errorf("error while listening: %w", err)
	}

	c.listenCancel = cancel
	c.listening.Store(true)

	return nil
}

// Next waits for the next notification and returns its payload.
func (c *Connection) Next(ctx context.Context) (string, error) {
	for {
		waitCtx, cancel := context.WithTimeout(ctx, KeepaliveCheckTime)
		notification, err := c.listenConn.WaitForNotification(waitCtx)

		cancel()

		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				pingCtx, cancel := context.WithTimeout(ctx, PingTimeout)
				pingErr := c.listenConn.Ping(pingCtx)

				cancel()

				if pingErr != nil {
					return "", fmt.Errorf("notification connection ping failed: %w", pingErr)
				}

				continue
			}

			return "", fmt.Errorf("unexpected error while waiting for a notification: %w", err)
		}

		if notification.Channel == c.channel {
			return notification.Payload, nil
		}
	}
}

// Notify sends a notification in the given transaction.
func (c *Connection) Notify(ctx context.Context, tx pgx.Tx, payload string) error {
	if _, err := tx.Exec(ctx, "SELECT pg_notify($1, $2);", c.channel, payload); err != nil {
		return fmt.Errorf("error while sending notification: %w", err)
	}

	return nil
}

// Run listens and calls handle for every notification.
func (c *Connection) Run(ctx context.Context, notify run.Notify, handle func(string) error) error {
	if err := c.Listen(ctx); err != nil {
		return err
	}

	notify.Notify()

	for {
		payload, err := c.Next(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil //nolint:nilerr
			}

			return err
		}

		if err := handle(payload); err != nil {
			return err
		}
	}
}

// Close closes the notification connection.
func (c *Connection) Close(ctx context.Context) error {
	if c.listenCancel != nil {
		c.listenCancel()
	}

	c.listening.Store(false)

	if err := c.listenConn.Close(ctx); err != nil {
		return fmt.Errorf("error while closing the notification connection: %w", err)
	}

	return nil
}
