package webdav

import (
	"log/slog"
	"net/http"

	"golang.org/x/net/webdav"

	"github.com/teapotovh/teapot/lib/httpauth"
	"github.com/teapotovh/teapot/service/files"
)

type WebDav struct {
	logger *slog.Logger

	files     *files.Files
	basicAuth *httpauth.BasicAuth
}

type WebDavConfig struct {
}

func NewWebDav(config WebDavConfig, logger *slog.Logger, files *files.Files) *WebDav {
	return &WebDav{
		logger: logger,

		files:     files,
		basicAuth: httpauth.NewBasicAuth(files.LDAPFactory(), httpauth.DefaultBasicAuthErrorHandler, logger.With("component", "auth")),
	}
}

func (wd *WebDav) Handler(prefix string) http.Handler {
	handler := http.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := httpauth.MustGetAuth(r)

		wd.logger.Info("got request with user", "user", user)

		// FileSystem: ,
		handler := &webdav.Handler{
			Prefix: prefix,
		}
		handler.ServeHTTP(w, r)
	}))

	handler = wd.basicAuth.Required(handler)
	handler = wd.basicAuth.Middleware(handler)
	return handler
}

// // TODO: use files for backends, start backends
// session, err := files.Sesssions().Get("luca")
// slog.Info("maybe got session", "session", session, "err", err)
//
// err = hpfs.WalkDir(session.FS(), ".", func(path string, d fs.DirEntry, err error) error {
// 	if err == nil {
// 		info, e := d.Info()
// 		slog.Info("walking", "path", path, "info", info, "err", e)
// 		return nil
// 	} else {
// 		return err
// 	}
// })
// if err != nil {
// 	slog.Error("error while walkinnn", "err", err)
