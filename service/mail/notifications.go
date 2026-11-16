package mail

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/teapotovh/teapot/lib/pgnotify"
	"github.com/teapotovh/teapot/lib/run"
)

const notificationsChannel = "mail_changes"

type notificationHub struct {
	mu     sync.Mutex
	nextID int
	subs   map[int]chan uuid.UUID
}

func newNotificationHub() *notificationHub {
	return &notificationHub{subs: map[int]chan uuid.UUID{}}
}

func (n *notificationHub) Subscribe() (<-chan uuid.UUID, func()) {
	n.mu.Lock()
	defer n.mu.Unlock()

	id := n.nextID
	n.nextID++

	ch := make(chan uuid.UUID, 1)
	n.subs[id] = ch

	return ch, func() {
		n.mu.Lock()
		defer n.mu.Unlock()

		if existing, ok := n.subs[id]; ok {
			delete(n.subs, id)
			close(existing)
		}
	}
}

func (n *notificationHub) Publish(mailboxID uuid.UUID) {
	n.mu.Lock()
	defer n.mu.Unlock()

	for _, ch := range n.subs {
		select {
		case ch <- mailboxID:
		default:
		}
	}
}

// Notifications manages mailbox change notifications for all mail components.
type Notifications struct {
	logger *slog.Logger
	conn   *pgnotify.Connection
	hub    *notificationHub
}

// NewNotifications creates a notification manager. An empty URL uses only the
// in-memory hub.
func NewNotifications(psqlURL string, logger *slog.Logger) (*Notifications, error) {
	notifications := &Notifications{
		logger: logger,
		hub:    newNotificationHub(),
	}

	if psqlURL == "" {
		return notifications, nil
	}

	config, err := pgx.ParseConfig(psqlURL)
	if err != nil {
		return nil, fmt.Errorf("error while parsing the psql notification URL: %w", err)
	}

	conn, err := pgnotify.NewConnection(config, notificationsChannel, logger)
	if err != nil {
		return nil, err
	}

	notifications.conn = conn

	return notifications, nil
}

// Subscribe returns a channel of mailbox changes. The caller must call the
// returned function to unsubscribe.
func (n *Notifications) Subscribe() (<-chan uuid.UUID, func()) {
	return n.hub.Subscribe()
}

// Publish sends a local notification.
func (n *Notifications) Publish(mailboxID uuid.UUID) {
	n.hub.Publish(mailboxID)
}

// Notify sends a notification in the given transaction.
func (n *Notifications) Notify(ctx context.Context, tx pgx.Tx, mailboxID uuid.UUID) error {
	if n.conn == nil {
		n.hub.Publish(mailboxID)
		return nil
	}

	return n.conn.Notify(ctx, tx, mailboxID.String())
}

// Run implements run.Runnable.
func (n *Notifications) Run(ctx context.Context, notify run.Notify) error {
	if n.conn == nil {
		notify.Notify()
		<-ctx.Done()

		return nil
	}

	return n.conn.Run(ctx, notify, func(payload string) error {
		mailboxID, err := uuid.Parse(payload)
		if err != nil {
			n.logger.WarnContext(ctx, "invalid mail notification payload", "payload", payload)
			return nil //nolint:nilerr
		}

		n.hub.Publish(mailboxID)

		return nil
	})
}
