package components

import (
	g "maragu.dev/gomponents"
	c "maragu.dev/gomponents/components"
	h "maragu.dev/gomponents/html"

	"github.com/teapotovh/teapot/lib/ui"
)

var CodeStyle = ui.MustParseStyle(`
	background: var(--theme-background-4);
`)

func Code(ctx ui.Context, children ...g.Node) g.Node {
	return h.Code(c.JoinAttrs("class", g.Group(children), ctx.Class(CodeStyle)))
}
