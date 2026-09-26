package wideevent

import (
	"log/slog"
	"net/http"
)

type HTTPWideEvent struct {
	logger *slog.Logger
}

func NewHTTPWideEvent(logger *slog.Logger) *HTTPWideEvent {
	return &HTTPWideEvent{
		logger: logger,
	}
}

func (hwe *HTTPWideEvent) Middleware(next http.Handler) http.Handler {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(WithLogger(r.Context(), hwe.logger))
		next.ServeHTTP(w, r)
	})

	return handler
}
