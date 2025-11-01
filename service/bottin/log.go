package bottin

import (
	"context"
	"log/slog"

	"github.com/teapotovh/teapot/lib/ldapserver"
)

// ContextHandler is a custom slog.Handler that prints values from
// special keys used by bottin found among Context values
type ContextHandler struct {
	handler slog.Handler
}

func NewContextHandler(h slog.Handler) *ContextHandler {
	return &ContextHandler{h}
}

func (h *ContextHandler) Handle(ctx context.Context, r slog.Record) error {
	if cID := ctx.Value(ldapserver.ContextKeyConnectionID); cID != nil {
		r.AddAttrs(slog.Int("client", cID.(int)))
	}
	if user := ctx.Value(ldapserver.ContextKeyUser); user != nil {
		r.AddAttrs(slog.Any("user", user.(User)))
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
