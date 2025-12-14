package webdav

import (
	"log/slog"
	"net/http"

	"github.com/rs/cors"
	"golang.org/x/net/webdav"

	"github.com/teapotovh/teapot/lib/httpauth"
	"github.com/teapotovh/teapot/service/files"
)

type WebDav struct {
	logger *slog.Logger

	files     *files.Files
	basicAuth *httpauth.BasicAuth

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
		basicAuth: httpauth.NewBasicAuth(
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
	logger := wd.logger.With("component", "webdav")

	handler := http.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := httpauth.MustGetAuth(r)

		session, err := wd.files.Sesssions().Get(auth.Username)
		if err != nil {
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}

		// TODO: perhaps specialize the logger here
		fs := newWebDavFSWrapper(session.FS(), logger)

		handler := &webdav.Handler{
			Prefix:     prefix,
			FileSystem: fs,
			// TODO: implement a custom locking system
			LockSystem: webdav.NewMemLS(),
			Logger: func(r *http.Request, err error) {
				logger.Error("error while handling WebDav request", "method", r.Method, "path", r.URL.Path, "err", err)
			},
		}

		handler.ServeHTTP(w, r)
	}))

	handler = wd.basicAuth.Required(handler)
	handler = wd.basicAuth.Middleware(handler)
	handler = wd.cors.Handler(handler)

	return handler
}
