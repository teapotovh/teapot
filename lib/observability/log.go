package observability

import (
	"context"
	"log/slog"

	"go.opentelemetry.io/otel/trace"
)

type Handler struct {
	handler slog.Handler
}

func (h *Handler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.handler.Enabled(ctx, level)
}

func (h *Handler) Handle(ctx context.Context, r slog.Record) error {
	spanCtx := trace.SpanContextFromContext(ctx)
	if !spanCtx.IsValid() {
		return nil
	}

	r.AddAttrs(
		slog.Bool("observability_is_sampled", spanCtx.IsSampled()),
		slog.String("observability_trace_id", spanCtx.TraceID().String()),
		slog.String("observability_span_id", spanCtx.SpanID().String()),
	)

	return h.handler.Handle(ctx, r)
}

func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &Handler{handler: h.handler.WithAttrs(attrs)}
}

func (h *Handler) WithGroup(name string) slog.Handler {
	return &Handler{handler: h.handler.WithGroup(name)}
}

// Ensure *Handler implements slog.Handler.
var _ slog.Handler = &Handler{}

// WithHandler wraps an existing logger to include tracing metadata from context values.
func WithHandler(logger *slog.Logger) *slog.Logger {
	return slog.New(&Handler{handler: logger.Handler()})
}
