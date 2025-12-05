package web

import (
	"fmt"
	"log/slog"
	"net/http"

	g "maragu.dev/gomponents"
	hx "maragu.dev/gomponents-htmx"
	h "maragu.dev/gomponents/html"

	"github.com/kataras/muxie"
	"github.com/teapotovh/teapot/lib/ui"
	"github.com/teapotovh/teapot/lib/ui/components"
	httpui "github.com/teapotovh/teapot/lib/ui/http"
)

type WebConfig struct {
	UI ui.UIConfig
}

type Web struct {
	logger *slog.Logger

	assetPath           string
	renderer            *ui.Renderer
	dependenciesHandler http.Handler
}

func NewWeb(config WebConfig, logger *slog.Logger) (*Web, error) {
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

func (web *Web) Register(mux *muxie.Mux) {
	mux.Handle(web.assetPath+"*", web.dependenciesHandler)
	mux.HandleFunc("/", web.Handle)
}

type HomePage struct {
}

func (hp HomePage) Render(ctx ui.Context) g.Node {
	return h.Div(
		g.Text("this is a test webpage, with a button"),
		components.Button(ctx,
			h.Class("primary"),
			hx.Target("/example"),
			g.Text("some default button"),
		),
		components.Button(ctx,
			h.Class("secondary"),
			hx.Target("/example"),
			g.Text("some secondary button"),
		),
	)
}

func (web *Web) Handle(w http.ResponseWriter, r *http.Request) {
	err := web.renderer.RenderPage(w, "test", HomePage{})
	if err != nil {
		web.logger.Error("error when rendering", "err", err)
	}
}
