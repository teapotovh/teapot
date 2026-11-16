package mail

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestNotificationsInMemory(t *testing.T) {
	t.Parallel()

	notifications, err := NewNotifications("", slog.New(slog.DiscardHandler))
	require.NoError(t, err)

	ch, unsubscribe := notifications.Subscribe()
	defer unsubscribe()

	first := uuid.New()
	notifications.Publish(first)

	select {
	case got := <-ch:
		require.Equal(t, first, got)
	case <-time.After(time.Second):
		t.Fatal("published notification was not received")
	}

	second := uuid.New()
	require.NoError(t, notifications.Notify(context.Background(), nil, second))

	select {
	case got := <-ch:
		require.Equal(t, second, got)
	case <-time.After(time.Second):
		t.Fatal("notified change was not received")
	}
}
