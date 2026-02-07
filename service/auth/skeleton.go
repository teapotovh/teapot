package auth

import (
	"net/http"

	g "maragu.dev/gomponents"

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
		components.Header(ctx,
			g.Group{components.HeaderTitle(ctx, g.Text(AppShort))},
			g.Group{},
		),
		components.Body(ctx, body),
	}
}
