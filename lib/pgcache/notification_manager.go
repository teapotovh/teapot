package pgcache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var (
	ErrClosing = errors.New("reconnecting connection is closing down")

	ConnectionKeepaliveCheckTime = 10 * time.Minute
	PingTimeout                  = 5 * time.Second
)

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

	listenConn, err := pgx.ConnectConfig(context.Background(), config)
	if err != nil {
		return nil, fmt.Errorf("error while opening listen connection: %w", err)
	}

	name := "teapot_invalidate_" + table
	nm := notificationManager[K]{
		logger: logger,

		id:         id,
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
		waitCtx, cancel := context.WithTimeout(ctx, ConnectionKeepaliveCheckTime)
		msg, err := nm.listenConn.WaitForNotification(waitCtx)

		cancel()

		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				pingCtx, cancel := context.WithTimeout(ctx, PingTimeout)
				err := nm.listenConn.Ping(pingCtx)

				cancel()

				if err != nil {
					return nil, fmt.Errorf("listen connection ping failed: %w", err)
				}

				continue // connection is alive; keep waiting
			}

			return nil, fmt.Errorf("unexpected error while waiting for notification: %w", err)
		}

		if msg.Channel == nm.channel {
			id, events, err := nm.decode(msg.Payload)
			if err != nil {
				return nil, fmt.Errorf("error while decoding notification payload: %w", err)
			}

			if id != nm.id {
				nm.logger.InfoContext(ctx, "received events", "id", nm.id, "events", len(events))
				return events, nil
			}
		}
	}
}

func (nm *notificationManager[K]) Notify(ctx context.Context, tx pgx.Tx, events []Event[K]) error {
	if len(events) == 0 {
		return nil
	}

	payload, err := nm.encode(events)
	if err != nil {
		return fmt.Errorf("error while encoding notification events: %w", err)
	}

	_, err = tx.Exec(ctx, "SELECT pg_notify($1, $2);", nm.channel, payload)
	if err != nil {
		return fmt.Errorf("error while sending notification: %w", err)
	}

	nm.logger.InfoContext(ctx, "sent notification", "id", nm.id, "events", len(events))

	return nil
}

func (nm *notificationManager[K]) Close(ctx context.Context) error {
	if nm.listenCancel != nil {
		nm.listenCancel()
	}

	if err := nm.listenConn.Close(ctx); err != nil {
		return fmt.Errorf("error while closing listen connection: %w", err)
	}

	return nil
}
