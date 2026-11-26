package webdav

import (
	"log/slog"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/cors"

	"github.com/teapotovh/teapot/lib/httpauth"
	libwebdav "github.com/teapotovh/teapot/lib/webdav"
	"github.com/teapotovh/teapot/service/files"
)

type WebDav struct {
	logger *slog.Logger

	files    *files.Files
	httpAuth *httpauth.BasicAuth

	cors *cors.Cors
}

type WebDavConfig struct{}

func NewWebDav(files *files.Files, config WebDavConfig, logger *slog.Logger) (*WebDav, error) {
	cors := cors.New(cors.Options{
		AllowedOrigins: []string{"*"},
		AllowedMethods: []string{
			http.MethodHead,
			http.MethodGet,
			http.MethodPost,
			http.MethodPut,
			http.MethodPatch,
			http.MethodDelete,
		},
		AllowedHeaders:   []string{"*"},
		AllowCredentials: true,
	})
	wd := WebDav{
		logger: logger,

		files: files,
		httpAuth: httpauth.NewBasicAuth(
			files.LDAPFactory(),
			httpauth.DefaultBasicAuthErrorHandler,
			logger.With("component", "auth"),
		),

		cors: cors,
	}

	return &wd, nil
}

// Handler implements httpsrv.HTTPService.
func (wd *WebDav) Handler(prefix string) http.Handler {
	handler := http.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := httpauth.MustGetAuth(r)

		session, err := wd.files.Session(r.Context(), auth.Username)
		if err != nil {
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}

		handler := &libwebdav.Handler{
			FileSystem: newFileSystem(session.FS()),
		}

		handler.ServeHTTP(w, r)
	}))

	handler = wd.httpAuth.Required(handler)
	handler = wd.httpAuth.Middleware(handler)
	handler = wd.cors.Handler(handler)

	return handler
}

// Metrics implements observability.Metrics.
func (wd *WebDav) Metrics() []prometheus.Collector {
	// The LDAP factory metrics are owned and exposed by files.Files.
	return wd.httpAuth.Metrics()
}
