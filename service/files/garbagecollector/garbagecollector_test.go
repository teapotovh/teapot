package garbagecollector

import (
	"context"
	"errors"
	"iter"
	"log/slog"
	"testing"
	"time"
)

var (
	errDeleteFailed = errors.New("delete failed")
	errRefsFailed   = errors.New("refs failed")
)

type fakeRefs struct {
	refs []string
	err  error
}

func (f *fakeRefs) LiveRefs(_ context.Context) ([]string, error) {
	return f.refs, f.err
}

func (f *fakeRefs) Close() {}

type fakeObjects struct {
	objects   []Object
	deleted   []string
	listErr   error
	deleteErr map[string]error
}

func (f *fakeObjects) List(_ context.Context) iter.Seq2[Object, error] {
	return func(yield func(Object, error) bool) {
		for _, object := range f.objects {
			if !yield(object, nil) {
				return
			}
		}

		if f.listErr != nil {
			yield(Object{}, f.listErr)
		}
	}
}

func (f *fakeObjects) Delete(_ context.Context, key string) error {
	if err := f.deleteErr[key]; err != nil {
		return err
	}

	f.deleted = append(f.deleted, key)

	return nil
}

func newTestCollector(refs Refs, objects Objects, gracePeriod time.Duration) *GarbageCollector {
	return newGarbageCollector(
		Config{GracePeriod: gracePeriod},
		refs,
		objects,
		slog.New(slog.DiscardHandler),
	)
}

func TestCollectRemovesUnreferencedOldObjects(t *testing.T) {
	t.Parallel()

	now := time.Now()
	old := now.Add(-time.Hour)
	fresh := now.Add(-time.Minute)

	refs := &fakeRefs{refs: []string{"live-old", "live-fresh"}}
	objects := &fakeObjects{objects: []Object{
		{Key: "live-old", ModTime: old, Size: 100},
		{Key: "live-fresh", ModTime: fresh, Size: 200},
		{Key: "garbage-old", ModTime: old, Size: 2048},
		{Key: "garbage-fresh", ModTime: fresh, Size: 4096},
	}}

	collector := newTestCollector(refs, objects, 10*time.Minute)

	stats, err := collector.collect(context.Background())
	if err != nil {
		t.Fatalf("collect: %v", err)
	}

	if len(objects.deleted) != 1 || objects.deleted[0] != "garbage-old" {
		t.Fatalf("deleted = %v, want [garbage-old]", objects.deleted)
	}

	if stats.deleted != 1 || stats.bytes != 2048 {
		t.Fatalf("stats = %+v, want 1 object and 2048 bytes", stats)
	}
}

func TestCollectContinuesAfterDeleteErrors(t *testing.T) {
	t.Parallel()

	now := time.Now()
	old := now.Add(-time.Hour)

	refs := &fakeRefs{}
	objects := &fakeObjects{
		objects: []Object{
			{Key: "first", ModTime: old, Size: 10},
			{Key: "second", ModTime: old, Size: 20},
			{Key: "third", ModTime: old, Size: 512},
		},
		deleteErr: map[string]error{
			"first":  errDeleteFailed,
			"second": errDeleteFailed,
		},
	}

	collector := newTestCollector(refs, objects, 10*time.Minute)

	stats, err := collector.collect(context.Background())
	if !errors.Is(err, errDeleteFailed) {
		t.Fatalf("collect error = %v, want %v", err, errDeleteFailed)
	}

	if len(objects.deleted) != 1 || objects.deleted[0] != "third" {
		t.Fatalf("deleted = %v, want [third]", objects.deleted)
	}

	if stats.deleted != 1 || stats.bytes != 512 {
		t.Fatalf("stats = %+v, want 1 object and 512 bytes", stats)
	}
}

func TestCollectFailsOnRefsError(t *testing.T) {
	t.Parallel()

	want := errRefsFailed
	refs := &fakeRefs{err: want}
	objects := &fakeObjects{}

	collector := newTestCollector(refs, objects, 10*time.Minute)

	stats, err := collector.collect(context.Background())
	if !errors.Is(err, want) {
		t.Fatalf("collect error = %v, want %v", err, want)
	}

	if stats.deleted != 0 || stats.bytes != 0 {
		t.Fatalf("stats = %+v, want 0 objects and 0 bytes", stats)
	}
}
