package mailtest

import (
	"context"
	"sync"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type Notifications struct {
	mu     sync.Mutex
	nextID int
	subs   map[int]chan uuid.UUID
}

func NewNotifications() *Notifications {
	return &Notifications{subs: map[int]chan uuid.UUID{}}
}

func (n *Notifications) Subscribe() (<-chan uuid.UUID, func()) {
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

func (n *Notifications) Publish(mailboxID uuid.UUID) {
	n.mu.Lock()
	defer n.mu.Unlock()

	for _, ch := range n.subs {
		select {
		case ch <- mailboxID:
		default:
		}
	}
}

func (n *Notifications) Notify(_ context.Context, _ pgx.Tx, mailboxID uuid.UUID) error {
	n.Publish(mailboxID)

	return nil
}
