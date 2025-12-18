package httplog

import (
	"context"
	"log/slog"
)

type contextKey string

const (
	RequestID     contextKey = "requestID"
	RequestMethod contextKey = "requestMethod"
	RequestURI    contextKey = "requestURI"
)

type Handler struct {
	handler slog.Handler
}

func (h *Handler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.handler.Enabled(ctx, level)
}

func (h *Handler) Handle(ctx context.Context, r slog.Record) error {
	if reqID, ok := ctx.Value(RequestID).(string); ok {
		r.AddAttrs(slog.String("requestid", reqID))
	}

	if reqID, ok := ctx.Value(RequestMethod).(string); ok {
		r.AddAttrs(slog.String("method", reqID))
	}

	if reqID, ok := ctx.Value(RequestURI).(string); ok {
		r.AddAttrs(slog.String("uri", reqID))
	}

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

// WithHandler wraps an existing logger to include HTTP metadata from context values.
func WithHandler(logger *slog.Logger) *slog.Logger {
	return slog.New(&Handler{handler: logger.Handler()})
}
