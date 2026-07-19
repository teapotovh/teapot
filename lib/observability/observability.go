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
	"go.opentelemetry.io/otel/trace"

	"github.com/teapotovh/teapot/lib/run"
)

const shutdownDelay = time.Second * 5

type ObservabilityConfig struct {
	Address string
	Tracing ObservabilityTracingConfig
}

type Observability struct {
	logger *slog.Logger

	inner    *http.Server
	mux      *http.ServeMux
	registry *prometheus.Registry

	collectors []prometheus.Collector

	prometheus *httpServicePrometheus
	pprof      *httpServicePProf
	readyz     *httpServiceZ
	livez      *httpServiceZ
	tracing    *tracing
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
	obs.pprof = &httpServicePProf{
		logger: logger.With("component", "pprof"),
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
	mux.Handle("/debug/{path...}", obs.pprof.Handler("/debug"))
	mux.Handle("/readyz", obs.readyz.Handler("/readyz"))
	mux.Handle("/ready/{name}", obs.readyz.Handler("/readyz"))
	mux.Handle("/livez", obs.livez.Handler("/livez"))
	mux.Handle("/live/{name}", obs.livez.Handler("/livez"))

	tracing, err := newTracing(config.Tracing, logger.With("component", "tracing"))
	if err != nil && !errors.Is(err, ErrTracingDisabled) {
		return nil, fmt.Errorf("error while configuring tracing: %w", err)
	}

	obs.tracing = tracing

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

type Tracing interface {
	// WithTracing provides the Tracer to a component
	WithTracing(tp trace.TracerProvider, tracer trace.Tracer)
}

// RegisterTracing registers a component to be traced.
func (obs *Observability) RegisterTracing(traceable Tracing) {
	if obs.tracing != nil {
		traceable.WithTracing(obs.tracing.tp, obs.tracing.tracer)
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
			type shutdown interface {
				Shutdown(ctx context.Context) error
			}

			type svc struct {
				shutdown

				name string
			}

			for _, svc := range []svc{{obs.inner, "observability"}, {obs.tracing, "tracing"}} {
				if svc.shutdown == nil {
					continue
				}

				ctx, cancel := context.WithTimeout(context.Background(), shutdownDelay)
				defer cancel()

				if err := svc.Shutdown(ctx); err != nil {
					return fmt.Errorf("error while shutting down observability %q: %w", svc.name, err)
				}
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
