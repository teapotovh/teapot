package web

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/kataras/muxie"
	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"

	"github.com/teapotovh/teapot/lib/ui"
	"github.com/teapotovh/teapot/lib/ui/components"
	httpui "github.com/teapotovh/teapot/lib/ui/http"
	"github.com/teapotovh/teapot/service/files"
)

type WebConfig struct {
	UI ui.UIConfig
}

type Web struct {
	dependenciesHandler http.Handler
	logger              *slog.Logger
	renderer            *ui.Renderer
	prefix              string
	assetPath           string
}

func NewWeb(files *files.Files, config WebConfig, logger *slog.Logger) (*Web, error) {
	renderer, err := ui.NewRenderer(config.UI.Renderer, ui.DefaultPage{}, logger.With("component", "renderer"))
	if err != nil {
		return nil, fmt.Errorf("error while constructing renderer: %w", err)
	}

	web := Web{
		logger: logger,

		assetPath:           config.UI.Renderer.AssetPath,
		renderer:            renderer,
		dependenciesHandler: httpui.ServeDependencies(renderer, logger.With("component", "dependencies")),
	}

	return &web, nil
}

func (web *Web) Handler(prefix string) http.Handler {
	web.prefix = prefix

	mux := muxie.NewMux()
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
	err := web.renderer.RenderPage(w, "test", HomePage{})
	if err != nil {
		web.logger.Error("error when rendering", "err", err)
	}
}
