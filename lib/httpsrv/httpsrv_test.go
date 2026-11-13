package httpsrv

import (
	"context"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(discardWriter{}, nil))
}

type discardWriter struct{}

func (discardWriter) Write(p []byte) (int, error) { return len(p), nil }

type stubNotify struct{}

func (stubNotify) Notify() {}

type handlerService struct{}

func (handlerService) Handler(prefix string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	return mux
}

// runServer starts the server in the background and returns a channel that
// receives the result of HTTPSrv.Run.
func runServer(t *testing.T, h *HTTPSrv, ctx context.Context) <-chan error {
	t.Helper()

	done := make(chan error, 1)
	go func() { done <- h.Run(ctx, stubNotify{}) }()

	return done
}

// TestHTTPSrvShutdownReturns ensures that after context cancellation the
// server shuts down gracefully and Run returns within the shutdown delay.
func TestHTTPSrvShutdownReturns(t *testing.T) {
	t.Parallel()

	h, err := NewHTTPSrv(HTTPSrvConfig{
		Address:       "127.0.0.1:18760",
		ShutdownDelay: 2 * time.Second,
	}, testLogger())
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	done := runServer(t, h, ctx)

	require.Eventually(t, h.running.Load, 5*time.Second, 10*time.Millisecond,
		"HTTPSrv did not start up in time")

	cancel()

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("HTTPSrv.Run did not return within 5s of cancellation: graceful shutdown hangs")
	}
}

// TestHTTPSrvServesRequests ensures that a request against a started server
// is answered with the registered handler, and that the server still shuts
// down gracefully afterwards.
func TestHTTPSrvServesRequests(t *testing.T) {
	t.Parallel()

	// fixed port so we can issue requests against it
	const addr = "127.0.0.1:18761"

	h, err := NewHTTPSrv(HTTPSrvConfig{
		Address:       addr,
		ShutdownDelay: time.Second,
	}, testLogger())
	require.NoError(t, err)

	h.Register("test", handlerService{}, "/")

	ctx, cancel := context.WithCancel(context.Background())
	done := runServer(t, h, ctx)

	require.Eventually(t, h.running.Load, 5*time.Second, 10*time.Millisecond,
		"HTTPSrv did not start up in time")

	resp, err := http.Get("http://" + addr + "/")
	require.NoError(t, err)

	t.Cleanup(func() { _ = resp.Body.Close() })

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	cancel()

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("HTTPSrv.Run did not return within 5s of cancellation: graceful shutdown hangs")
	}
}
