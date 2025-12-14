package http

import (
	"log/slog"
	"net/http"

	"github.com/teapotovh/teapot/lib/ui"
	"github.com/teapotovh/teapot/lib/ui/dependency"
)

func ServeDependencies(renderer *ui.Renderer, logger *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		dep, bytes, err := renderer.Dependency(r.URL.Path)
		if err != nil {
			logger.Debug("invalid dependency request", "err", err)
		}

		logger.Debug("serving dependency", "dependency", dep, "bytes", len(bytes))

		ct := "text/plain"

		switch dep.Type {
		case dependency.DependencyTypeStyle:
			ct = "text/css"
		case dependency.DependencyTypeScript:
			ct = "application/javascript"
		}

		w.Header().Add("Content-Type", ct)
		w.WriteHeader(http.StatusOK)

		l, err := w.Write(bytes)
		if err != nil {
			logger.Error("error while serving dependency", "dependency", dep, "err", err)
		} else if l != len(bytes) {
			logger.Warn("not all bytes have been sent while serving dependency", "dependency", dep)
		}
	})
}
