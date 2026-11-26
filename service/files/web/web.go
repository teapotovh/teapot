package web

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/teapotovh/teapot/lib/httplog"
	"github.com/teapotovh/teapot/lib/ui"
	"github.com/teapotovh/teapot/lib/webauth"
	"github.com/teapotovh/teapot/lib/webhandler"
	"github.com/teapotovh/teapot/lib/wellknown"
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
	wellKnown  wellknown.WellKnown
}

func NewWeb(filesService *files.Files, config WebConfig, logger *slog.Logger) (*Web, error) {
	httpLog, err := httplog.NewHTTPLog(config.HTTPLog, logger.With("component", "httplog"))
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

	webAuth, err := webauth.NewWebAuth(filesService.LDAPFactory(), config.WebAuth, webauth.WebAuthOptions{
		LoginPath:  PathLogin,
		LogoutPath: PathLogout,
		ReturnPath: webauth.ConstantPath(PathBrowse),
		App:        App,
	}, logger.With("component", "auth"))
	if err != nil {
		return nil, fmt.Errorf("error while constructing webauth: %w", err)
	}

	wellKnown := wellknown.WellKnown{
		wellknown.Webdav: wellknown.ResolverHostPath(files.WebDavPrefix),
	}

	web := Web{
		logger: logger,

		files: filesService,

		httpLog:    httpLog,
		webHandler: webHandler,
		webAuth:    webAuth,
		wellKnown:  wellKnown,
	}

	return &web, nil
}

// Handler implements httpsrv.HTTPService.
func (web *Web) Handler(prefix string) http.Handler {
	mux := http.NewServeMux()

	mux.Handle(wellknown.Prefix, web.wellKnown)

	mux.Handle(web.webHandler.AssetPath, web.webHandler.AssetHandler)

	mux.Handle(PathLogin, web.webHandler.Adapt(web.webAuth.Login))
	mux.Handle(PathLogout, web.webHandler.Adapt(web.webAuth.Logout))

	mux.Handle(PathBrowseAt("{path...}"), web.webHandler.Adapt(web.Browse))
	mux.Handle(PathBrowseDialogOf("{dialog}"), web.webHandler.Adapt(web.BrowseDialog))
	mux.Handle(PathDownloadAt("{path...}"), web.webHandler.AdaptHTTP(web.Download))
	mux.Handle(PathRenameAt("{path...}"), web.webHandler.Adapt(web.Rename))
	mux.Handle(PathDeleteAt("{path...}"), web.webHandler.Adapt(web.Delete))

	mux.Handle("/{path...}", web.webHandler.Adapt(web.NotFound))

	var handler http.Handler = mux

	handler = web.webAuth.Middleware(handler)
	handler = web.httpLog.LogMiddleware(handler)
	handler = web.httpLog.ExtractMiddleware(handler)

	return handler
}

func (web *Web) NotFound(w http.ResponseWriter, r *http.Request) (ui.Component, error) {
	if r.URL.Path == PathIndex {
		return web.Index(w, r)
	}

	return nil, webhandler.ErrNotFound
}
