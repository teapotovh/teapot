package pagedbtree

import (
	"context"
	"io"
	"os"
	"sync"
	"testing"
	"time"
)

type memoryFileSystem struct {
	file *memoryFile
}

func (f memoryFileSystem) OpenFile(string, int, os.FileMode) (File, error) {
	return f.file, nil
}

type memoryFile struct {
	mu       sync.Mutex
	data     []byte
	writeErr error
}

type memoryFileInfo struct {
	size int64
}

func (i memoryFileInfo) Name() string       { return "memory" }
func (i memoryFileInfo) Size() int64        { return i.size }
func (i memoryFileInfo) Mode() os.FileMode  { return 0o600 }
func (i memoryFileInfo) ModTime() time.Time { return time.Time{} }
func (i memoryFileInfo) IsDir() bool        { return false }
func (i memoryFileInfo) Sys() any           { return nil }

func newMemoryFile() *memoryFile {
	return &memoryFile{}
}

func (f *memoryFile) ReadAt(p []byte, offset int64) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if offset < 0 {
		return 0, os.ErrInvalid
	}

	if offset >= int64(len(f.data)) {
		return 0, io.EOF
	}

	n := copy(p, f.data[offset:])
	if n < len(p) {
		return n, io.EOF
	}

	return n, nil
}

func (f *memoryFile) WriteAt(p []byte, offset int64) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.writeErr != nil {
		return 0, f.writeErr
	}

	end := offset + int64(len(p))
	if end > int64(len(f.data)) {
		data := make([]byte, end)
		copy(data, f.data)
		f.data = data
	}

	copy(f.data[offset:], p)

	return len(p), nil
}

func (f *memoryFile) Truncate(size int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if size < 0 {
		return os.ErrInvalid
	}

	data := make([]byte, size)
	copy(data, f.data)
	f.data = data

	return nil
}

func (f *memoryFile) Stat() (os.FileInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	return memoryFileInfo{size: int64(len(f.data))}, nil
}

func (f *memoryFile) Close() error {
	return nil
}

func (f *memoryFile) setWriteError(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.writeErr = err
}

func (f *memoryFile) corrupt(offset int) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.data[offset] ^= 0xff
}

func (f *memoryFile) bytes() []byte {
	f.mu.Lock()
	defer f.mu.Unlock()

	return append([]byte(nil), f.data...)
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

func stopTestStore(store *Bytes) {
	_ = store.wait()

	if state := store.state.Load(); state != nil {
		state.cancel()
	}

	<-store.done
}

func waitTestStore(t *testing.T, store *Bytes) {
	t.Helper()

	if err := store.wait(); err != nil {
		t.Fatal(err)
	}
}

func runTestStore(t *testing.T, store *Bytes) {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	notify := testNotify{ch: make(chan struct{}, 1)}

	go func() {
		done <- store.Run(ctx, notify)
	}()

	select {
	case <-notify.ch:
	case <-time.After(5 * time.Second):
		t.Fatal("store did not start")
	}

	t.Cleanup(func() {
		cancel()

		if err := <-done; err != nil {
			t.Errorf("Run() error: %v", err)
		}
	})
}

func startTestStore(t *testing.T, cfg Config, file *memoryFile) (*Bytes, <-chan error) {
	t.Helper()

	store := newTestBytes(t, cfg, file)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	notify := testNotify{ch: make(chan struct{}, 1)}

	go func() {
		done <- store.Run(ctx, notify)
	}()

	select {
	case <-notify.ch:
	case <-time.After(5 * time.Second):
		t.Fatal("store did not start")
	}

	t.Cleanup(func() {
		cancel()
		<-store.done
	})

	return store, done
}
