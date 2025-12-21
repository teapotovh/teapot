package httpsrv

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"path/filepath"
	"time"

	"github.com/kataras/muxie"
	"github.com/kataras/requestid"

	"github.com/teapotovh/teapot/lib/run"
)

type HTTPSrvConfig struct {
	Address       string
	ShutdownDelay time.Duration
}

type HTTPSrv struct {
	logger        *slog.Logger
	inner         *http.Server
	mux           *muxie.Mux
	shutdownDelay time.Duration
}

func NewHTTPSrv(config HTTPSrvConfig, logger *slog.Logger) (*HTTPSrv, error) {
	mux := muxie.NewMux()
	mux.Use(func(h http.Handler) http.Handler { return requestid.Handler(h) })

	inner := http.Server{
		Handler:           mux,
		ReadHeaderTimeout: time.Minute,
		Addr:              config.Address,
	}

	return &HTTPSrv{
		logger: logger,

		shutdownDelay: config.ShutdownDelay,
		inner:         &inner,
		mux:           mux,
	}, nil
}

type HTTPService interface {
	// Handler returns an http.Handler for that service being rooted at `prefix`.
	Handler(prefix string) http.Handler
}

func (h *HTTPSrv) Register(name string, service HTTPService, prefix string) {
	handler := service.Handler(prefix)
	h.logger.Info("registering HTTP service", "name", name, "prefix", prefix)
	h.mux.Handle(filepath.Join(prefix, "*"), handler)
}

// Run implements run.Runnable.
func (h *HTTPSrv) Run(ctx context.Context, notify run.Notify) error {
	var ch chan error
	defer close(ch)

	h.inner.BaseContext = func(l net.Listener) context.Context { return ctx }

	go func() {
		h.logger.Info("opening HTTP server", "address", h.inner.Addr)
		notify.Notify()

		if err := h.inner.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
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
