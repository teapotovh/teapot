package pgcache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync/atomic"

	"github.com/jackc/pgx/v5"

	"github.com/teapotovh/teapot/lib/pgnotify"
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

	// nm is a thin wrapper around PostgreSQL LISTEN and NOTIFY.
	nm         *pgnotify.Manager
	channel    string
	fromString FromString[K]
	listening  atomic.Bool
}

func newNotificationManager[K Key[K]](
	config *pgx.ConnConfig,
	table string,
	fromString FromString[K],
	logger *slog.Logger,
) (*notificationManager[K], error) {
	nm, err := pgnotify.NewManager(config, logger.With("component", "pgnotify"))
	if err != nil {
		return nil, fmt.Errorf("error while building notification manager: %w", err)
	}

	name := "teapot_invalidate_" + table
	manager := notificationManager[K]{
		logger: logger,

		nm:         nm,
		channel:    name,
		fromString: fromString,
	}

	return &manager, nil
}

// This is keyed by key (string) so if we have multiple updates for a single key,
// they are merged into the latest one received. For example, if we have
// "key1: store, delete, store, store" it results in store, which will
// invalidate caches on other watchers and have them re-fetch the updated value.
// This implies that the ordering of events has to match the order in which they
// were applied to the database in the trasaction.
type keymap map[string]EvenType

type payload struct {
	KeyMap keymap `json:"k"`
}

func (nm *notificationManager[K]) encode(events []Event[K]) ([]byte, error) {
	serialized := keymap{}

	for _, event := range events {
		k := event.Key.String()
		serialized[k] = event.Type
	}

	payload := payload{
		KeyMap: serialized,
	}

	bytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("error while marshaling keys: %w", err)
	}

	return bytes, nil
}

func (nm *notificationManager[K]) decode(raw []byte) ([]Event[K], error) {
	var payload payload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("error while unmarshaling keys: %w", err)
	}

	var events []Event[K]

	for str, t := range payload.KeyMap {
		key, err := nm.fromString(str)
		if err != nil {
			return nil, fmt.Errorf("error while building key from encoded %q: %w", str, err)
		}

		events = append(events, Event[K]{
			Type: t,
			Key:  *key,
		})
	}

	return events, nil
}

func (nm *notificationManager[K]) IsListening() bool {
	return nm.listening.Load()
}

func (nm *notificationManager[K]) Listen(ctx context.Context) error {
	if err := nm.nm.Listen(ctx, nm.channel); err != nil {
		nm.listening.Store(false)

		return fmt.Errorf("error while listening: %w", err)
	}

	nm.listening.Store(true)

	return nil
}

func (nm *notificationManager[K]) Next(ctx context.Context) ([]Event[K], error) {
	for {
		channel, raw, err := nm.nm.Next(ctx)
		if err != nil {
			nm.listening.Store(false)

			return nil, fmt.Errorf("unexpected error while waiting for notification: %w", err)
		}

		if channel != nm.channel {
			continue
		}

		events, err := nm.decode(raw)
		if err != nil {
			return nil, fmt.Errorf("error while decoding notification payload: %w", err)
		}

		nm.logger.InfoContext(ctx, "received events", "events", len(events))

		return events, nil
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

	if err := nm.nm.Notify(ctx, tx, nm.channel, payload); err != nil {
		return fmt.Errorf("error while sending notification: %w", err)
	}

	nm.logger.InfoContext(ctx, "sent notification", "events", len(events))

	return nil
}

func (nm *notificationManager[K]) Close() error {
	nm.listening.Store(false)

	if err := nm.nm.Close(); err != nil {
		return fmt.Errorf("error while closing listen connection: %w", err)
	}

	return nil
}
