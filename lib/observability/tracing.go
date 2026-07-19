package observability

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.41.0"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type ObservabilityTracingConfig struct {
	Endpoint       string
	ServiceName    string
	SampleRate     float32
	ConnectTimeout time.Duration
}

type tracing struct {
	tp     *sdktrace.TracerProvider
	tracer trace.Tracer
}

func newTracing(config ObservabilityTracingConfig, logger *slog.Logger) (*tracing, error) {
	if len(config.Endpoint) <= 0 {
		return nil, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), config.ConnectTimeout)
	defer cancel()
	conn, err := grpc.NewClient(config.Endpoint, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("error while creating grpc client for tracing: %w", err)
	}

	exp, err := otlptracegrpc.New(ctx, otlptracegrpc.WithGRPCConn(conn))
	if err != nil {
		return nil, fmt.Errorf("error while creating otel grpc client: %w", err)
	}

	res, err := resource.New(ctx, resource.WithAttributes(semconv.ServiceName(config.ServiceName)))
	if err != nil {
		return nil, fmt.Errorf("could not create otel app resource :%w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.AlwaysSample())),
	)
	tracer := tp.Tracer(config.ServiceName)

	logger.Info("configured and enabled opentelemetry tracing", "endpoint", config.Endpoint)

	return &tracing{
		tp:     tp,
		tracer: tracer,
	}, nil
}

func (t tracing) Shutdown(ctx context.Context) error {
	if err := t.tp.Shutdown(ctx); err != nil {
		return fmt.Errorf("error while flushing otel traces: %w", err)
	}
	return nil
}

type tracerKeyType struct{}

var (
	tracerKey = tracerKeyType{}

	NoopTracer = noop.NewTracerProvider().Tracer("noop")
)

func ContextWithTracer(ctx context.Context, tracer trace.Tracer) context.Context {
	return context.WithValue(ctx, tracerKey, tracer)
}

func TracerFromContext(ctx context.Context) trace.Tracer {
	if t, ok := ctx.Value(tracerKey).(trace.Tracer); ok {
		return t
	}

	return NoopTracer
}

func SpanEnd(span trace.Span, err error) error {
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	span.End()
	return err
}
