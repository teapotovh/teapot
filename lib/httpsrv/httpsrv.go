package httpsrv

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"path"
	"time"

	"github.com/kataras/muxie"

	"github.com/teapotovh/teapot/lib/run"
)

type HttpSrvConfig struct {
	Address       string
	ShutdownDelay time.Duration
}

type HttpSrv struct {
	logger *slog.Logger

	shutdownDelay time.Duration
	inner         *http.Server
	mux           *muxie.Mux
}

func NewHttpSrv(config HttpSrvConfig, logger *slog.Logger) (*HttpSrv, error) {
	mux := muxie.NewMux()
	inner := http.Server{
		Handler: mux,
		Addr:    config.Address,
	}
	return &HttpSrv{
		logger: logger,

		shutdownDelay: config.ShutdownDelay,
		inner:         &inner,
		mux:           mux,
	}, nil
}

type HttpService interface {
	// Handler returns an http.Handler for that service being rooted at `prefix`.
	// The provided argument is the prefix prepended to each HTTP request path.
	Handler(string) http.Handler
}

func (h *HttpSrv) Register(name string, service HttpService, prefix string) {
	handler := service.Handler(prefix)
	h.logger.Info("registering HTTP service", "name", name, "prefix", prefix)
	h.mux.Handle(path.Join(prefix, "*"), handler)
}

// Run implements run.Runnable
func (h *HttpSrv) Run(ctx context.Context, notify run.Notify) error {
	var ch chan error
	defer close(ch)

	h.inner.BaseContext = func(l net.Listener) context.Context { return ctx }

	go func() {
		h.logger.Info("opening HTTP server", "address", h.inner.Addr)
		notify.Notify()
		if err := h.inner.ListenAndServe(); err != http.ErrServerClosed {
			ch <- err
		} else {
			ch <- nil
		}
	}()

	for {
		select {
		case <-ctx.Done():
			ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(h.shutdownDelay))
			defer cancel()

			if err := h.inner.Shutdown(ctx); err != nil {
				return fmt.Errorf("error while shutting down the HTTP server: %w", err)
			}
			return <-ch
		case err := <-ch:
			if err != nil {
				return fmt.Errorf("error while running the HTTP server: %w", err)
			}
			return nil
		}
	}
}
