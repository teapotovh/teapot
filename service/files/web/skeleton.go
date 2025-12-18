package web

import (
	"net/http"

	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"

	"github.com/teapotovh/teapot/lib/ui"
	"github.com/teapotovh/teapot/lib/ui/components"
)

type skeleton struct {
	body ui.Component
}

func Skeleton(r *http.Request, component ui.Component) (ui.Component, error) {
	return skeleton{
		body: component,
	}, nil
}

func (skeleton skeleton) Render(ctx ui.Context) g.Node {
	body := skeleton.body.Render(ctx)

	return g.Group{
		components.Header(ctx, g.Group{
			components.HeaderTitle(ctx, h.Href("/"), g.Text("Files")),
		}, g.Group{
			components.HeaderLink(ctx, h.Href("/login"), g.Text("login")),
			components.HeaderLink(ctx, h.Href("/register"), g.Text("register")),
		}),
		components.Body(ctx, body),
	}
}
