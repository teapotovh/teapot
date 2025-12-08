package components

import (
	g "maragu.dev/gomponents"
	c "maragu.dev/gomponents/components"

	"github.com/teapotovh/teapot/lib/ui"
)

var GridStyle = ui.MustParseStyle(`
	display: grid;
	grid-template-columns: 1fr 3fr 2fr 6fr 2fr 3fr 1fr;
`)

type GridComponent = func(opts ...g.Node) g.Node

func Grid(ctx ui.Context, component GridComponent, opts ...g.Node) g.Node {
	return component(c.JoinAttrs("class", g.Group(opts), ctx.Class(GridStyle)))
}

var ClampStyle = ui.MustParseStyle(`
	grid-column-start: 1;
	grid-column-end: 8;

	@media (min-width: 1024px) {
		& {
			grid-column-start: 2;
			grid-column-end: 7;
		}
	}
	@media (min-width: 1440px) {
		& {
			grid-column-start: 3;
			grid-column-end: 6;
		}
	}
`)

func Clamp(ctx ui.Context, component GridComponent, opts ...g.Node) g.Node {
	return component(c.JoinAttrs("class", g.Group(opts), ctx.Class(ClampStyle)))
}

var OutsideLeftStyle = ui.MustParseStyle(`
	grid-column-start: 1;
	grid-column-end: 2;

	@media (min-width: 1440px) {
		& {
			grid-column-start: 2;
			grid-column-end: 3;
		}
	}
`)

func OutsideLeft(ctx ui.Context, component GridComponent, opts ...g.Node) g.Node {
	return component(c.JoinAttrs("class", g.Group(opts), ctx.Class(OutsideLeftStyle)))
}
