package store

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"
)

func TestWatchLive(t *testing.T) {
	t.Parallel()

	store, ctx := newTestStore(t)
	fs := store.For(2000)

	if err := fs.Mkdir(ctx, "/a"); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}

	if err := fs.Put(ctx, "/a/f", bytes.NewBufferString("x")); err != nil {
		t.Fatalf("Put: %v", err)
	}

	revision := store.Revision()
	watchCtx, cancel := context.WithCancel(ctx)
	events := make(chan Event, 16)
	errs := make(chan error, 1)

	go func() {
		errs <- store.Watch(watchCtx, revision, func(event Event) error {
			events <- event

			return nil
		})
	}()

	time.Sleep(10 * time.Millisecond)

	if err := fs.Put(ctx, "/a/g", bytes.NewBufferString("y")); err != nil {
		t.Fatalf("Put: %v", err)
	}

	event := receiveEvent(t, events)
	if event.Inode.Name != "g" || event.Inode.Revision != revision+1 {
		t.Fatalf("live event = %#v, want g at revision %d", event.Inode, revision+1)
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

func TestWatchRequiresCurrentRevision(t *testing.T) {
	t.Parallel()

	store, ctx := newTestStore(t)
	fs := store.For(2001)

	if err := fs.Mkdir(ctx, "/a"); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}

	if err := fs.Put(ctx, "/a/f", bytes.NewBufferString("x")); err != nil {
		t.Fatalf("Put: %v", err)
	}

	revision := store.Revision()

	if err := store.Watch(ctx, revision-1, func(Event) error { return nil }); !errors.Is(err, ErrRevisionTooOld) {
		t.Fatalf("Watch after old revision error = %v, want %v", err, ErrRevisionTooOld)
	}

	watchCtx, cancel := context.WithCancel(ctx)
	events := make(chan Event, 1)

	go func() {
		_ = store.Watch(watchCtx, revision, func(event Event) error {
			events <- event

			return nil
		})
	}()

	time.Sleep(10 * time.Millisecond)

	if err := fs.Delete(ctx, "/a/f"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	event := receiveEvent(t, events)
	if event.Inode.Name != "f" || !event.Inode.Deleted || event.Inode.Revision != revision+1 {
		t.Fatalf("delete event = %#v, want delete for f at revision %d", event.Inode, revision+1)
	}

	cancel()
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
