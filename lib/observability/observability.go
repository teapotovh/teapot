package observability

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/teapotovh/teapot/lib/run"
)

const shutdownDelay = time.Second * 5

type ObservabilityConfig struct {
	Address string
}

type Observability struct {
	logger *slog.Logger

	inner    *http.Server
	mux      *http.ServeMux
	registry *prometheus.Registry

	collectors []prometheus.Collector

	prometheus *httpServicePrometheus
	readyz     *httpServiceZ
	livez      *httpServiceZ
}

func NewObservability(config ObservabilityConfig, logger *slog.Logger) (*Observability, error) {
	mux := http.NewServeMux()
	inner := http.Server{
		Handler:           mux,
		ReadHeaderTimeout: time.Minute,
		Addr:              config.Address,
	}

	obs := Observability{
		logger: logger,

		inner:    &inner,
		mux:      mux,
		registry: prometheus.NewRegistry(),
	}

	obs.prometheus = &httpServicePrometheus{
		logger:   logger.With("component", "prometheus"),
		registry: obs.registry,
	}
	obs.readyz = &httpServiceZ{
		logger: logger.With("component", "readyz"),
		name:   "readyz",
		checks: map[string]Check{},
	}
	obs.livez = &httpServiceZ{
		logger: logger.With("component", "livez"),
		name:   "livez",
		checks: map[string]Check{},
	}

	mux.Handle("/metrics", obs.prometheus.Handler("/metrics"))
	mux.Handle("/readyz", obs.readyz.Handler("/readyz"))
	mux.Handle("/ready/{name}", obs.readyz.Handler("/readyz"))
	mux.Handle("/livez", obs.livez.Handler("/livez"))
	mux.Handle("/live/{name}", obs.livez.Handler("/livez"))

	return &obs, nil
}

type Metrics interface {
	// Metrics returns all metrics exposed by an application.
	// Calls to this method should be idempotent and return consistent values
	// throughout the lifetime of an object. In particular, the pointers for the
	// returned `prometheus.Collector`s should not change over time.
	Metrics() []prometheus.Collector
}

// RegisterMetrics gets all the collectors from the Metrics method on the
// provided subsystem. These collectors will then be registered to be exported
// via the metrics endpoint.
func (obs *Observability) RegisterMetrics(metrics Metrics) {
	obs.collectors = append(obs.collectors, metrics.Metrics()...)
}

type Check interface {
	// Runs a check within the given timeframe provided by the context.
	// If the check is successful, return nil, otherwise return an error
	// describing what failed.
	Check(ctx context.Context) error
}

type CheckFunc func(ctx context.Context) error

func (cf CheckFunc) Check(ctx context.Context) error {
	return cf(ctx)
}

type ReadinessChecks interface {
	// ReadinessChecks returns all the checks for readiness supported by this object.
	ReadinessChecks() map[string]Check
}

// RegisterReadyz registers a named check for readiness.
func (obs *Observability) RegisterReadyz(readiness ReadinessChecks) {
	for name, check := range readiness.ReadinessChecks() {
		if old, ok := obs.readyz.checks[name]; ok {
			obs.logger.Warn("redefined readiness check", "name", name, "old", old, "new", check)
		}

		obs.readyz.checks[name] = check
	}
}

type LivenessChecks interface {
	// LivenessChecks returns all the checks for liveness supported by this object.
	LivenessChecks() map[string]Check
}

// RegisterLivez registers a named check for liveness.
func (obs *Observability) RegisterLivez(liveness LivenessChecks) {
	for name, check := range liveness.LivenessChecks() {
		if old, ok := obs.livez.checks[name]; ok {
			obs.logger.Warn("redefined liveness check", "name", name, "old", old, "new", check)
		}

		obs.livez.checks[name] = check
	}
}

// Run implements run.Runnable.
func (obs *Observability) Run(ctx context.Context, notify run.Notify) error {
	if err := obs.registerMetrics(); err != nil {
		return fmt.Errorf("error while registering all metrics: %w", err)
	}

	var ch chan error
	defer close(ch)

	obs.inner.BaseContext = func(l net.Listener) context.Context { return ctx }

	go func() {
		obs.logger.Info("opening observability server", "address", obs.inner.Addr)
		notify.Notify()

		if err := obs.inner.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			ch <- err
		} else {
			ch <- nil
		}
	}()

	for {
		select {
		case <-ctx.Done():
			ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(shutdownDelay))
			defer cancel()

			if err := obs.inner.Shutdown(ctx); err != nil {
				return fmt.Errorf("error while shutting down the observability server: %w", err)
			}

			return <-ch
		case err := <-ch:
			if err != nil {
				return fmt.Errorf("error while running the observability server: %w", err)
			}

			return nil
		}
	}
}

func (obs *Observability) registerMetrics() error {
	var errs []error

	for _, collector := range obs.collectors {
		if err := obs.registry.Register(collector); err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}
