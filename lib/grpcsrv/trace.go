package grpcsrv

import (
	"context"

	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"

	"github.com/teapotovh/teapot/lib/observability"
)

func tracerUnaryServerInterceptor(tracer trace.Tracer) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		ctx = observability.ContextWithTracer(ctx, tracer)
		return handler(ctx, req)
	}
}

func tracerStreamServerInterceptor(tracer trace.Tracer) grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		wrapped := &wrappedStream{ServerStream: ss, ctx: observability.ContextWithTracer(ss.Context(), tracer)}
		return handler(srv, wrapped)
	}
}

type wrappedStream struct {
	grpc.ServerStream

	ctx context.Context
}

func (w *wrappedStream) Context() context.Context { return w.ctx }
