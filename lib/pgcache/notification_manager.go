package pgcache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"

	"github.com/cenkalti/backoff/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

var ErrClosing = errors.New("reconnecting connection is closing down")

type unit struct{}

type ReconnectingConn struct {
	ctx     context.Context
	mu      sync.Mutex
	closing bool
	conn    *pgx.Conn

	config    *pgx.ConnConfig
	onConnect func(ctx context.Context, conn *pgx.Conn) error
}

const reconnectingConnMaxRetries = 2

func (rc *ReconnectingConn) Do[T any](
	ctx context.Context,
	fn func(ctx context.Context, conn *pgx.Conn) (T, error),
) (T, error) {
	var empty T

	f := func() (T, error) {
		rc.mu.Lock()
		conn := rc.conn
		closing := rc.closing
		rc.mu.Unlock()

		if closing {
			return empty, ErrClosing
		}

		if conn == nil || conn.IsClosed() {
			var err error

			conn, err = rc.reconnect(ctx, rc.ctx)
			if err != nil {
				return empty, fmt.Errorf("reconnecting closed connection: %w", err)
			}
		}

		return fn(ctx, conn)
	}

	return backoff.Retry(
		ctx,
		f,
		backoff.WithMaxTries(reconnectingConnMaxRetries),
		backoff.WithBackOff(expoBackoff),
	)
}

func (rc *ReconnectingConn) Close(ctx context.Context) error {
	rc.mu.Lock()
	rc.closing = true

	conn := rc.conn
	defer rc.mu.Unlock()

	if conn != nil {
		return conn.Close(ctx)
	}

	return nil
}

func (rc *ReconnectingConn) reconnect(reqCtx, connCtx context.Context) (*pgx.Conn, error) {
	rc.mu.Lock()
	defer rc.mu.Unlock()

	if rc.closing {
		return nil, ErrClosing
	}

	if rc.conn != nil {
		if err := rc.conn.Close(reqCtx); err != nil {
			return nil, fmt.Errorf("closing previous connection: %w", err)
		}
	}

	conn, err := pgx.ConnectConfig(connCtx, rc.config)
	if err != nil {
		return nil, fmt.Errorf("(re)connecting: %w", err)
	}

	if rc.onConnect != nil {
		if err = rc.onConnect(reqCtx, conn); err != nil {
			return nil, fmt.Errorf("running on-connect boostrap: %w", err)
		}
	}

	rc.conn = conn

	return conn, nil
}

var ErrUnexpectedEvenType = errors.New("unexpected event type")

type EvenType uint8

const (
	EventTypeStore EvenType = iota
	EventTypeDelete
)

func (et EvenType) String() string {
	switch et {
	case EventTypeStore:
		return "store"
	case EventTypeDelete:
		return "delete"
	default:
		return "unknown"
	}
}

type Event[K Key[K]] struct {
	Type EvenType
	Key  K
}

func (e Event[K]) String() string {
	return "(" + e.Type.String() + ", " + e.Key.String() + ")"
}

type notificationManager[K Key[K]] struct {
	logger *slog.Logger

	id uuid.UUID
	// notifyConn is a single connection used to send notifications for cache invalidations.
	notifyConn *ReconnectingConn
	// listenConn is a single connection used to receive notifications for cache invalidations.
	listenConn   *pgx.Conn
	listenCancel context.CancelFunc
	channel      string
	fromString   FromString[K]
	listening    atomic.Bool
}

func newNotificationManager[K Key[K]](
	config *pgx.ConnConfig,
	table string,
	fromString FromString[K],
	logger *slog.Logger,
) (*notificationManager[K], error) {
	id, err := uuid.NewRandom()
	if err != nil {
		return nil, fmt.Errorf("error while generating uuid: %w", err)
	}

	notifyConn := &ReconnectingConn{ctx: context.Background(), config: config}

	listenConn, err := pgx.ConnectConfig(context.Background(), config)
	if err != nil {
		return nil, fmt.Errorf("error while opening listen connection: %w", err)
	}

	name := "teapot_invalidate_" + table
	nm := notificationManager[K]{
		logger: logger,

		id:         id,
		notifyConn: notifyConn,
		listenConn: listenConn,
		channel:    name,
		fromString: fromString,
	}

	return &nm, nil
}

// This is keyed by key (string) so if we have multiple updates for a single key,
// they are merged into the latest one received. For example, if we have
// "key1: store, delete, store, store" it results in store, which will
// invalidate caches on other watchers and have them re-fetch the updated value.
// This implies that the ordering of events has to match the order in which they
// were applied to the database in the trasaction.
type keymap map[string]EvenType

type payload struct {
	ID     uuid.UUID `json:"i"`
	KeyMap keymap    `json:"k"`
}

func (nm *notificationManager[K]) encode(events []Event[K]) (string, error) {
	serialized := keymap{}

	for _, event := range events {
		k := event.Key.String()
		serialized[k] = event.Type
	}

	payload := payload{
		ID:     nm.id,
		KeyMap: serialized,
	}

	bytes, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("error while marshaling keys: %w", err)
	}

	return string(bytes), nil
}

func (nm *notificationManager[K]) decode(raw string) (uuid.UUID, []Event[K], error) {
	var payload payload
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return uuid.UUID{}, nil, fmt.Errorf("error while unmarshaling keys: %w", err)
	}

	var events []Event[K]

	for str, t := range payload.KeyMap {
		key, err := nm.fromString(str)
		if err != nil {
			return uuid.UUID{}, nil, fmt.Errorf("error while building key from encoded %q: %w", str, err)
		}

		events = append(events, Event[K]{
			Type: t,
			Key:  *key,
		})
	}

	return payload.ID, events, nil
}

func (nm *notificationManager[K]) IsListening() bool {
	return nm.listening.Load()
}

func (nm *notificationManager[K]) Ping(ctx context.Context) error {
	// NOTE: we cannot ping nm.listenConn as it will be busy performing LISTEN
	_, err := nm.notifyConn.Do(ctx, func(ctx context.Context, conn *pgx.Conn) (unit, error) {
		return unit{}, conn.Ping(ctx)
	})
	if err != nil {
		return fmt.Errorf("error while pinging notify connection: %w", err)
	}

	return nil
}

func (nm *notificationManager[K]) Listen(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)

	_, err := nm.listenConn.Exec(ctx, "LISTEN "+nm.channel+";")
	if err != nil {
		defer cancel()
		return fmt.Errorf("error while listening: %w", err)
	}

	nm.listenCancel = cancel
	nm.listening.Store(true)

	return nil
}

func (nm *notificationManager[K]) Next(ctx context.Context) ([]Event[K], error) {
	for {
		msg, err := nm.listenConn.WaitForNotification(ctx)
		if err != nil {
			return nil, err
		}

		if msg.Channel == nm.channel {
			id, events, err := nm.decode(msg.Payload)
			if err != nil {
				return nil, fmt.Errorf("error while decoding notification payload: %w", err)
			}

			// We are only interested in events NOT coming from this same cache
			if id != nm.id {
				nm.logger.Info("received events", "id", nm.id, "events", len(events))
				return events, nil
			}
		}
	}
}

func (nm *notificationManager[K]) Notify(ctx context.Context, events []Event[K]) error {
	if len(events) == 0 {
		return nil
	}

	payload, err := nm.encode(events)
	if err != nil {
		return fmt.Errorf("error while encoding notification events: %w", err)
	}

	_, err = nm.notifyConn.Do(ctx, func(ctx context.Context, conn *pgx.Conn) (pgconn.CommandTag, error) {
		return conn.Exec(ctx, "SELECT pg_notify($1, $2);", nm.channel, payload)
	})
	if err != nil {
		return fmt.Errorf("error while sending notification: %w", err)
	}

	nm.logger.Info("sent notification", "id", nm.id, "events", len(events))

	return nil
}

func (nm *notificationManager[K]) Close(ctx context.Context) error {
	if nm.listenCancel != nil {
		nm.listenCancel()
	}

	if err := nm.notifyConn.Close(ctx); err != nil {
		return fmt.Errorf("error while closing notify connection: %w", err)
	}

	if err := nm.listenConn.Close(ctx); err != nil {
		return fmt.Errorf("error while closing listen connection: %w", err)
	}

	return nil
}
