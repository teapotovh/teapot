package web

import (
	"fmt"
	"log/slog"
	"net/http"
	"path"

	"github.com/kataras/muxie"

	"github.com/teapotovh/teapot/lib/httplog"
	"github.com/teapotovh/teapot/lib/ui"
	"github.com/teapotovh/teapot/lib/webauth"
	"github.com/teapotovh/teapot/lib/webhandler"
	"github.com/teapotovh/teapot/service/files"
)

type WebConfig struct {
	HTTPLog    httplog.HTTPLogConfig
	WebHandler webhandler.WebHandlerConfig
	WebAuth    webauth.WebAuthConfig
}

type Web struct {
	logger *slog.Logger

	files *files.Files

	httpLog    *httplog.HTTPLog
	webHandler *webhandler.WebHandler
	webAuth    *webauth.WebAuth
}

func NewWeb(files *files.Files, config WebConfig, logger *slog.Logger) (*Web, error) {
	// Provide request information in all log operations
	logger = httplog.WithHandler(logger)

	httpLog, err := httplog.NewHTTPLog(config.HTTPLog, logger)
	if err != nil {
		return nil, fmt.Errorf("error while constructing httplog: %w", err)
	}

	webHandler, err := webhandler.NewWebHandler(
		config.WebHandler,
		Skeleton,
		webhandler.DefaultErrorHandlers,
		logger.With("component", "webhandler"),
	)
	if err != nil {
		return nil, fmt.Errorf("error while constructing webhandler: %w", err)
	}

	webAuth, err := webauth.NewWebAuth(files.LDAPFactory(), config.WebAuth, webauth.WebAuthOptions{
		LoginPath:  PathLogin,
		LogoutPath: PathLogout,
		ReturnPath: PathIndex,
		App:        App,
	}, logger.With("component", "auth"))
	if err != nil {
		return nil, fmt.Errorf("error while constructing webauth: %w", err)
	}

	web := Web{
		logger: logger,

		files: files,

		httpLog:    httpLog,
		webHandler: webHandler,
		webAuth:    webAuth,
	}

	return &web, nil
}

// Handler implements httpsrv.HTTPService.
func (web *Web) Handler(prefix string) http.Handler {
	mux := muxie.NewMux()
	mux.Use(web.httpLog.ExtractMiddleware)
	mux.Use(web.httpLog.LogMiddleware)
	mux.Use(web.webAuth.Middleware)

	mux.Handle(web.webHandler.AssetPath, web.webHandler.AssetHandler)

	mux.Handle(PathLogin, web.webHandler.Adapt(web.webAuth.Login))
	mux.Handle(PathLogout, web.webHandler.Adapt(web.webAuth.Logout))

	mux.Handle(PathIndex, web.webHandler.Adapt(web.Index))
	mux.Handle(path.Join(PathBrowse, "*"), web.webHandler.Adapt(web.Browse))

	mux.Handle("/*", web.webHandler.Adapt(web.NotFound))

	return mux
}

func (web *Web) NotFound(w http.ResponseWriter, r *http.Request) (ui.Component, error) {
	return nil, webhandler.ErrNotFound
}
