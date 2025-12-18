package http

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/teapotovh/teapot/lib/httphandler"
	"github.com/teapotovh/teapot/lib/ui"
	"github.com/teapotovh/teapot/lib/ui/dependency"
)

func ServeDependencies(renderer *ui.Renderer, logger *slog.Logger) httphandler.HTTPHandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) error {
		dep, bytes, err := renderer.Dependency(r.URL.Path)
		if err != nil {
			return fmt.Errorf("invalid dependency request: %w", err)
		}

		logger.Debug("serving dependency", "dependency", dep, "bytes", len(bytes))

		ct := "text/plain"

		switch dep.Type {
		case dependency.DependencyTypeStyle:
			ct = "text/css"
		case dependency.DependencyTypeScript:
			ct = "application/javascript"
		case dependency.DependencyTypeInvalid:
		default:
			err = fmt.Errorf("could not serve: %w", dependency.ErrInvalidDependencyType)
			return httphandler.NewInternalError(err, nil)
		}

		w.Header().Add("Content-Type", ct)

		return httphandler.Write(w, http.StatusOK, bytes)
	}
}
