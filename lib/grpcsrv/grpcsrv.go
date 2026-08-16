package grpcsrv

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"sync/atomic"
	"time"

	"github.com/grpc-ecosystem/go-grpc-middleware/providers/prometheus"
	vtcodec "github.com/planetscale/vtprotobuf/codec/grpc"
	"github.com/teapotovh/teapot/lib/run"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/encoding"
	_ "google.golang.org/grpc/encoding/proto"
	"google.golang.org/grpc/grpclog"
)

type GRPCSrvConfig struct {
	Address       string
	ShutdownDelay time.Duration
}

type GRPCSrv struct {
	logger *slog.Logger

	inner   *grpc.Server
	running atomic.Bool
	metrics *prometheus.ServerMetrics
	tp      trace.TracerProvider
	tracer  trace.Tracer

	address       string
	shutdownDelay time.Duration
	services      []GRPCService
}

func NewGRPCSrv(config GRPCSrvConfig, logger *slog.Logger) (*GRPCSrv, error) {
	// Unfortunately, gRPC uses a bunch of global variables for configuration.
	// To enable using VT proto marshaling and slog logging, we have to set these
	// global variables.
	grpclog.SetLoggerV2(&grpcLogger{logger: logger})
	encoding.RegisterCodec(vtcodec.Codec{})

	metrics := prometheus.NewServerMetrics()

	g := GRPCSrv{
		logger: logger,

		metrics: metrics,

		address:       config.Address,
		shutdownDelay: config.ShutdownDelay,
	}

	return &g, nil
}

type GRPCService interface {
	// Register registers the service with the provided *grpc.Server
	Register(server *grpc.Server)
}

func (g *GRPCSrv) Register(name string, service GRPCService) {
	g.logger.Info("registering gRPC service", "name", name)
	g.services = append(g.services, service)
}

// runWithTimeout runs a function with the provided timeout, and
// returns true if the timeout was triggered
func runWithTimeout(timeout time.Duration, fn func()) bool {
	done := make(chan struct{}, 1)
	defer close(done)

	go func() {
		fn()
		done <- struct{}{}
	}()

	select {
	case <-done:
		return false
	case <-time.After(timeout):
		return true
	}
}

// Run implements run.Runnable.
func (g *GRPCSrv) Run(ctx context.Context, notify run.Notify) (err error) {
	g.inner = grpc.NewServer(
		grpc.StreamInterceptor(g.metrics.StreamServerInterceptor()),
		grpc.UnaryInterceptor(g.metrics.UnaryServerInterceptor()),
		grpc.ChainUnaryInterceptor(tracerUnaryServerInterceptor(g.tracer)),
		grpc.ChainStreamInterceptor(tracerStreamServerInterceptor(g.tracer)),
		grpc.StatsHandler(otelgrpc.NewServerHandler(
			otelgrpc.WithTracerProvider(g.tp),
		)),
	)

	for _, service := range g.services {
		service.Register(g.inner)
	}

	lis, err := net.Listen("tcp", g.address)
	if err != nil {
		return fmt.Errorf("failed to listen on %q: %v", g.address, err)
	}
	defer func() {
		if e := lis.Close(); e != nil {
			err = errors.Join(err, e)
		}
	}()

	var ch chan error
	defer close(ch)

	go func() {
		g.logger.Info("opening gRPC server", "address", g.address)
		g.running.Store(true)
		notify.Notify()

		if err := g.inner.Serve(lis); err != nil {
			ch <- err
		}

		g.running.Store(false)
	}()

	for {
		select {
		case <-ctx.Done():
			if runWithTimeout(g.shutdownDelay, g.inner.GracefulStop) {
				g.logger.Warn("graceful shutdown timed out, force stopping the gRPC server")
			}

			g.inner.Stop()
			g.running.Store(false)

			return nil
		case err := <-ch:
			return fmt.Errorf("error while running the gRPC server: %w", err)
		}
	}
}

// WithTracing implements observability.Tracing.
func (g *GRPCSrv) WithTracing(tp trace.TracerProvider, tracer trace.Tracer) {
	g.tp = tp
	g.tracer = tracer
}
