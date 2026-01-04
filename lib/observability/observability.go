package observability

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/teapotovh/teapot/lib/httpsrv"
	"github.com/teapotovh/teapot/lib/run"
)

const (
	namePrometheus = "prometheus"
	nameReadyz     = "readyz"
	nameLivez      = "livez"

	prefixPrometheus = "/metrics"
	prefixReadyz     = "/readyz"
	prefixLivez      = "/livez"

	shutdownDelay = time.Second * 5
)

type ObservabilityConfig struct {
	Address string
}

type Observability struct {
	logger *slog.Logger

	inner    *httpsrv.HTTPSrv
	registry *prometheus.Registry

	collectors []prometheus.Collector

	prometheus *httpServicePrometheus
	readyz     *httpServiceZ
	livez      *httpServiceZ
}

func NewObservability(config ObservabilityConfig, logger *slog.Logger) (*Observability, error) {
	inner, err := httpsrv.NewHTTPSrv(httpsrv.HTTPSrvConfig{
		Address:       config.Address,
		ShutdownDelay: shutdownDelay,
	}, logger)
	if err != nil {
		return nil, fmt.Errorf("could not initialize inner httpsrv for observability: %w", err)
	}

	obs := Observability{
		logger: logger,

		inner:    inner,
		registry: prometheus.NewRegistry(),
	}
	obs.prometheus = &httpServicePrometheus{
		logger:   logger.With("component", namePrometheus),
		registry: obs.registry,
	}
	obs.readyz = &httpServiceZ{
		logger: logger.With("component", nameReadyz),
		name:   nameReadyz,
		checks: map[string]Check{},
	}
	obs.livez = &httpServiceZ{
		logger: logger.With("component", nameLivez),
		name:   nameLivez,
		checks: map[string]Check{},
	}

	inner.Register(namePrometheus, obs.prometheus, prefixPrometheus)
	inner.Register(nameReadyz, obs.readyz, prefixReadyz)
	inner.Register(nameLivez, obs.livez, prefixLivez)

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

type LivelinessChecks interface {
	// LivelinessChecks returns all the checks for liveliness supported by this object.
	LivelinessChecks() map[string]Check
}

// RegisterLivez registers a named check for liveliness.
func (obs *Observability) RegisterLivez(liveliness LivelinessChecks) {
	for name, check := range liveliness.LivelinessChecks() {
		if old, ok := obs.livez.checks[name]; ok {
			obs.logger.Warn("redefined liveliness check", "name", name, "old", old, "new", check)
		}

		obs.livez.checks[name] = check
	}
}

// Run implements run.Runnable.
func (obs *Observability) Run(ctx context.Context, notify run.Notify) error {
	if err := obs.registerMetrics(); err != nil {
		return fmt.Errorf("error while registering all metrics: %w", err)
	}

	return obs.inner.Run(ctx, notify)
}

func (obs *Observability) registerMetrics() error {
	var errs []error

	for _, collector := range obs.collectors {
		if err := prometheus.Register(collector); err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}
