package web

import (
	"net/http"

	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"

	"github.com/teapotovh/teapot/lib/ui"
	"github.com/teapotovh/teapot/lib/ui/components"
	"github.com/teapotovh/teapot/lib/webauth"
)

type skeleton struct {
	auth *webauth.Auth
	body ui.Component
}

func Skeleton(r *http.Request, component ui.Component) (ui.Component, error) {
	auth := webauth.GetAuth(r)

	return skeleton{
		body: component,
		auth: auth,
	}, nil
}

func (skeleton skeleton) Render(ctx ui.Context) g.Node {
	body := skeleton.body.Render(ctx)

	var login g.Node
	if skeleton.auth != nil {
		login = g.Group{
			h.Div(g.Textf("Hi %s!", skeleton.auth.Username)),
			components.HeaderLink(ctx, h.Href(PathLogout), g.Text("Logout")),
		}
	} else {
		login = components.HeaderLink(ctx, h.Href(PathLogin), g.Text("Login"))
	}

	return g.Group{
		components.Header(ctx,
			g.Group{components.HeaderTitle(ctx, h.Href(PathIndex), g.Text(AppShort))},
			g.Group{login},
		),
		components.Body(ctx, body),
	}
}
