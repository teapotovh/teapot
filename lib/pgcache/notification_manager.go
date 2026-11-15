package pgcache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/teapotovh/teapot/lib/pgnotify"
)

var (
	ErrUnexpectedEvenType = errors.New("unexpected event type")
)

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

	id         uuid.UUID
	channel    string
	conn       *pgnotify.Connection
	fromString FromString[K]
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

	channel := "teapot_invalidate_" + table

	conn, err := pgnotify.NewConnection(config, channel, logger)
	if err != nil {
		return nil, err
	}

	return &notificationManager[K]{
		logger:     logger,
		id:         id,
		channel:    channel,
		conn:       conn,
		fromString: fromString,
	}, nil
}

// This is keyed by key (string) so if we have multiple updates for a single key,
// they are merged into the latest one received. For example, if we have
// "key1: store, delete, store, store" it results in store, which will
// invalidate caches on other watchers and have them re-fetch the updated value.
// This implies that the ordering of events has to match the order in which they
// were applied to the database in the transaction.
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

func (nm *notificationManager[K]) decode(raw string, fromString FromString[K]) (uuid.UUID, []Event[K], error) {
	var payload payload
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return uuid.UUID{}, nil, fmt.Errorf("error while unmarshaling keys: %w", err)
	}

	var events []Event[K]

	for str, t := range payload.KeyMap {
		key, err := fromString(str)
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
	return nm.conn.IsListening()
}

func (nm *notificationManager[K]) Listen(ctx context.Context) error {
	return nm.conn.Listen(ctx)
}

func (nm *notificationManager[K]) Next(ctx context.Context) ([]Event[K], error) {
	for {
		raw, err := nm.conn.Next(ctx)
		if err != nil {
			return nil, err
		}

		id, events, err := nm.decode(raw, nm.fromString)
		if err != nil {
			return nil, fmt.Errorf("error while decoding notification payload: %w", err)
		}

		if id != nm.id {
			nm.logger.InfoContext(ctx, "received events", "id", nm.id, "events", len(events))

			return events, nil
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

	if err := nm.conn.Notify(ctx, tx, payload); err != nil {
		return err
	}

	nm.logger.InfoContext(ctx, "sent notification", "id", nm.id, "events", len(events))

	return nil
}

func (nm *notificationManager[K]) Close(ctx context.Context) error {
	return nm.conn.Close(ctx)
}
