package web

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/teapotovh/teapot/lib/run"
	"github.com/teapotovh/teapot/service/files/store"
)

func TestWriteTarGz(t *testing.T) {
	t.Parallel()

	storage, ctx := newTestStore(t)
	fs := storage.For(9001)

	if err := fs.Mkdir(ctx, "/dir"); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}

	if err := fs.Put(ctx, "/dir/a.txt", strings.NewReader("hello")); err != nil {
		t.Fatalf("Put: %v", err)
	}

	if err := fs.Put(ctx, "/b.txt", strings.NewReader("world")); err != nil {
		t.Fatalf("Put: %v", err)
	}

	var buf bytes.Buffer

	if err := writeTarGz(&buf, ctx, fs, []string{"/dir", "/b.txt"}); err != nil {
		t.Fatalf("writeTarGz: %v", err)
	}

	gzipReader, err := gzip.NewReader(&buf)
	if err != nil {
		t.Fatalf("NewReader: %v", err)
	}

	defer func() {
		if err := gzipReader.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	}()

	archive := tar.NewReader(gzipReader)
	got := map[string]string{}

	for {
		header, err := archive.Next()
		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			t.Fatalf("Next: %v", err)
		}

		data, err := io.ReadAll(archive)
		if err != nil {
			t.Fatalf("ReadAll: %v", err)
		}

		got[header.Name] = string(data)
	}

	want := map[string]string{
		"dir/":      "",
		"dir/a.txt": "hello",
		"b.txt":     "world",
	}

	if len(got) != len(want) {
		t.Fatalf("archive entries = %v, want %v", got, want)
	}

	for name, content := range want {
		if got[name] != content {
			t.Errorf("archive entry %q = %q, want %q", name, got[name], content)
		}
	}
}

type readyService chan struct{}

func (s readyService) Run(step run.Step) error {
	select {
	case s <- struct{}{}:
	default:
	}

	step.Notify()

	<-step.Context().Done()

	return nil
}

func newTestStore(t *testing.T) (*store.Store, context.Context) {
	t.Helper()

	logger := slog.New(slog.DiscardHandler)

	storage, err := store.NewMem(logger)
	if err != nil {
		t.Fatalf("NewMem: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	ready := make(readyService, 1)
	errs := make(chan error, 1)

	runner := run.NewRun(run.RunConfig{Timeout: 10 * time.Second}, logger)
	runner.Add("store", storage, nil)
	runner.Add("ready", ready, nil)

	go func() {
		errs <- runner.Run(ctx)
	}()

	select {
	case <-ready:
	case err := <-errs:
		cancel()
		t.Fatalf("Run: %v", err)
	case <-time.After(10 * time.Second):
		cancel()
		t.Fatal("store did not become ready")
	}

	t.Cleanup(func() {
		cancel()

		select {
		case err := <-errs:
			if err != nil {
				t.Errorf("Run returned an error: %v", err)
			}
		case <-time.After(10 * time.Second):
			t.Error("store did not stop")
		}
	})

	return storage, ctx
}
