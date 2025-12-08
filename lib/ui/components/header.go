package components

import (
	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"

	"github.com/teapotovh/teapot/lib/ui"
)

var HeaderStyle = ui.MustParseStyle(`
	height: var(--size-8);
	border-bottom: 1px solid var(--theme-wireframe1);
	width: 100vw;

	.logo {
	  flex-direction: row;
	  justify-content: flex-end;
	  align-items: center;
	  padding-right: var(--size-4);

	  display: none;
	  @media (min-width: 1024px) {
	    display: flex;
	  }
	}

	.clamp {
	  display: flex;
	  flex-direction: row;
	  justify-content: space-between;

	  div {
	    display: flex;
	    flex-direction: row;
	    align-items: center;
	  }
	  
	  .left { justify-content: flex-start; }
	  .right { justify-content: flex-end; }
	}
`)

func Header(ctx ui.Context, left g.Node, right g.Node) g.Node {
	return Grid(ctx, h.Nav, ctx.Class(HeaderStyle),
		OutsideLeft(ctx, h.Div, h.Class("logo"), Logo(ctx)),
		Clamp(ctx, h.Main, h.Class("clamp"), ctx.Class(WireframeStyle),
			h.Div(h.Class("left"), left),
			h.Div(h.Class("right"), right),
		),
	)
}
