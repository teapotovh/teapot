package store

import (
	"bytes"
	"context"
	"testing"
	"time"
)

func TestWatchCatchUpAndLive(t *testing.T) {
	t.Parallel()

	store, ctx := newTestStore(t)
	fs := store.For(2000)

	if err := fs.Mkdir(ctx, "/a"); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}

	if err := fs.Put(ctx, "/a/f", bytes.NewBufferString("x"), 0o644); err != nil {
		t.Fatalf("Put: %v", err)
	}

	watchCtx, cancel := context.WithCancel(ctx)
	events := make(chan Event, 16)

	errs := make(chan error, 1)
	go func() {
		errs <- store.Watch(watchCtx, 0, func(event Event) error {
			events <- event

			return nil
		})
	}()

	last := uint64(0)

	for range 3 {
		event := receiveEvent(t, events)
		if event.Inode.Revision <= last {
			t.Fatalf("revision %d is not after %d", event.Inode.Revision, last)
		}

		last = event.Inode.Revision
	}

	if err := fs.Put(ctx, "/a/g", bytes.NewBufferString("y"), 0o644); err != nil {
		t.Fatalf("Put: %v", err)
	}

	event := receiveEvent(t, events)
	if event.Inode.Name != "g" {
		t.Fatalf("live event name = %q, want g", event.Inode.Name)
	}

	cancel()

	select {
	case err := <-errs:
		if err != nil {
			t.Fatalf("Watch: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("watch did not stop")
	}
}

func receiveEvent(t *testing.T, events <-chan Event) Event {
	t.Helper()

	select {
	case event := <-events:
		return event
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for a watch event")

		return Event{}
	}
}
