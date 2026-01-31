package components

import (
	g "maragu.dev/gomponents"
	c "maragu.dev/gomponents/components"
	h "maragu.dev/gomponents/html"

	"github.com/teapotovh/teapot/lib/ui"
)

var WireframeStyle = ui.MustParseStyle(`
	padding-left: var(--size-1);
	padding-right: var(--size-1);

	@media (min-width: 1024px) {
	  padding-left: var(--size-2);
	  padding-right: var(--size-2);
	}

	@media (min-width: 1440px) {
	  padding-left: var(--size-4);
	  padding-right: var(--size-4);
		border-left: 1px dashed var(--theme-wireframe-0);
		border-right: 1px dashed var(--theme-wireframe-0);
	}
`)

var HorizontalLineStyle = ui.MustParseStyle(`
	height: 0px;
	width: 100%;

	margin: var(--size-2) 0;
	border-bottom: 1px dashed var(--theme-wireframe-0);
`)

func HorizontalLine(ctx ui.Context, children ...g.Node) g.Node {
	return h.Div(c.JoinAttrs("class", g.Group(children), ctx.Class(HorizontalLineStyle)))
}
