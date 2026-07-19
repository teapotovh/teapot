package ldapsrv

import (
	"context"
	"log/slog"

	"github.com/google/uuid"
)

// ContextHandler is a custom slog.Handler that prints values from
// special keys used by bottin found among Context values.
type ContextHandler struct {
	handler slog.Handler
}

func NewContextHandler(h slog.Handler) *ContextHandler {
	return &ContextHandler{h}
}

func (h *ContextHandler) Handle(ctx context.Context, r slog.Record) error {
	if addr := ctx.Value(ContextKeyAddr); addr != nil {
		r.AddAttrs(slog.String("addr", addr.(string)))
	}

	if requestID := ctx.Value(ContextKeyRequestID); requestID != nil {
		r.AddAttrs(slog.String("requestid", requestID.(uuid.UUID).String()))
	}

	return h.handler.Handle(ctx, r)
}

func (h *ContextHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.handler.Enabled(ctx, level)
}

func (h *ContextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &ContextHandler{handler: h.handler.WithAttrs(attrs)}
}

func (h *ContextHandler) WithGroup(name string) slog.Handler {
	return &ContextHandler{handler: h.handler.WithGroup(name)}
}
