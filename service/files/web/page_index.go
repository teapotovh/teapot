package web

import (
	"net/http"

	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"

	"github.com/teapotovh/teapot/lib/ui"
	"github.com/teapotovh/teapot/lib/webhandler"
)

type index struct{}

func (i index) Render(ctx ui.Context) g.Node {
	return h.H1(g.Text("index"))
}

// Ensure index implements ui.Component.
var _ ui.Component = index{}

func (web *Web) Index(w http.ResponseWriter, r *http.Request) (ui.Component, error) {
	return webhandler.NewPage("index", "index page for files", index{}), nil
}
