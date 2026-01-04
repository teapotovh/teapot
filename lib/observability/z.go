package observability

import (
	"log/slog"
	"net/http"
)

type httpServiceZ struct {
	logger *slog.Logger

	checks map[string]Check
}

// Handler implements httpsrv.Handler
func (r *httpServiceZ) Handler(prefix string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	})
}
