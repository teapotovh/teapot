package pagedbtree

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/teapotovh/teapot/lib/fs"
)

type memoryFileSystem struct {
	file *memoryFile
}

func (f memoryFileSystem) OpenFile(string, int, os.FileMode) (fs.File, error) {
	return f.file, nil
}

type memoryFile = fs.MemoryFile

func newMemoryFile() *memoryFile {
	return fs.NewMemoryFile()
}

func openTestStore(t *testing.T, cfg Config) (*Bytes, *memoryFile) {
	t.Helper()

	file := newMemoryFile()

	return openTestStoreWithFile(t, cfg, file), file
}

func openTestStoreWithFile(t *testing.T, cfg Config, file *memoryFile) *Bytes {
	t.Helper()

	store := newTestBytes(t, cfg, file)
	runTestStore(t, store)

	return store
}

func newTestBytes(t *testing.T, cfg Config, file *memoryFile) *Bytes {
	t.Helper()

	store, err := Open(memoryFileSystem{file: file}, cfg)
	if err != nil {
		t.Fatal(err)
	}

	return store
}

type testStoreRun struct {
	cancel context.CancelFunc
	done   <-chan error

	once sync.Once
	err  error
}

func (r *testStoreRun) stop() error {
	r.once.Do(func() {
		r.cancel()
		r.err = <-r.done
	})

	return r.err
}

var testStoreRuns sync.Map

func stopTestStore(store *Bytes) {
	value, ok := testStoreRuns.Load(store)
	if !ok {
		return
	}

	// The map stores *testStoreRun only.
	_ = value.(*testStoreRun).stop()
}

func runTestStore(t *testing.T, store *Bytes) {
	t.Helper()

	startTestRun(t, store, true)
}

func startTestStore(t *testing.T, cfg Config, file *memoryFile) (*Bytes, <-chan error) {
	t.Helper()

	store := newTestBytes(t, cfg, file)
	done := startTestRun(t, store, false)

	return store, done
}

func startTestRun(t *testing.T, store *Bytes, report bool) <-chan error {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	runDone := make(chan error, 1)
	done := make(chan error, 1)
	notify := testNotify{ctx: ctx, ch: make(chan unit, 1)}

	go func() {
		err := store.Run(notify)
		runDone <- err

		done <- err
	}()

	select {
	case <-notify.ch:
	case <-time.After(5 * time.Second):
		cancel()
		t.Fatal("store did not start")
	}

	run := &testStoreRun{cancel: cancel, done: runDone}
	testStoreRuns.Store(store, run)

	t.Cleanup(func() {
		err := run.stop()
		if report && err != nil {
			t.Errorf("Run() error: %v", err)
		}

		testStoreRuns.Delete(store)
	})

	return done
}
