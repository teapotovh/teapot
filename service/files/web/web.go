package web

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/kataras/muxie"
	g "maragu.dev/gomponents"
	c "maragu.dev/gomponents/components"
	h "maragu.dev/gomponents/html"

	"github.com/teapotovh/teapot/lib/httpauth"
	"github.com/teapotovh/teapot/lib/ui"
	"github.com/teapotovh/teapot/lib/ui/components"
	httpui "github.com/teapotovh/teapot/lib/ui/http"
	"github.com/teapotovh/teapot/service/files"
)

type WebConfig struct {
	UI      ui.UIConfig
	JWTAuth httpauth.JWTAuthConfig
}

type Web struct {
	logger *slog.Logger

	files *files.Files

	dependenciesHandler http.Handler
	renderer            *ui.Renderer
	assetPath           string

	auth *httpauth.JWTAuth
}

func NewWeb(files *files.Files, config WebConfig, logger *slog.Logger) (*Web, error) {
	renderer, err := ui.NewRenderer(config.UI.Renderer, logger.With("component", "renderer"))
	if err != nil {
		return nil, fmt.Errorf("error while constructing renderer: %w", err)
	}

	web := Web{
		logger: logger,

		files: files,

		assetPath:           config.UI.Renderer.AssetPath,
		renderer:            renderer,
		dependenciesHandler: httpui.ServeDependencies(renderer, logger.With("component", "dependencies")),

		auth: httpauth.NewJWTAuth(files.LDAPFactory(), config.JWTAuth, logger.With("component", "auth")),
	}

	return &web, nil
}

// Handler implements httpsrv.HTTPService.
func (web *Web) Handler(prefix string) http.Handler {
	mux := muxie.NewMux()
	mux.Use(web.auth.Middleware)

	mux.Handle(web.assetPath+"*", web.dependenciesHandler)
	mux.HandleFunc("/", web.Handle)

	return mux
}

type HomePage struct{}

func (hp HomePage) Render(ctx ui.Context) g.Node {
	return g.Group{
		components.Header(ctx, g.Group{
			components.HeaderTitle(ctx, h.Href("/"), g.Text("Files")),
		}, g.Group{
			components.HeaderLink(ctx, h.Href("/login"), g.Text("login")),
			components.HeaderLink(ctx, h.Href("/register"), g.Text("register")),
		}),
		components.Body(ctx,
			g.Text("this is a test webpage, with a button"),
		),
	}
}

func (web *Web) Handle(w http.ResponseWriter, r *http.Request) {
	err := web.renderer.RenderPage(w, c.HTML5Props{
		Title:       "index",
		Language:    "en",
		Description: "index page for files",
	}, HomePage{})
	if err != nil {
		web.logger.Error("error when rendering", "err", err)
	}
}
