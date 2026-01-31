package components

import (
	g "maragu.dev/gomponents"
	hx "maragu.dev/gomponents-htmx"
	c "maragu.dev/gomponents/components"
	h "maragu.dev/gomponents/html"

	"github.com/teapotovh/teapot/lib/ui"
)

var HeaderStyle = ui.MustParseStyle(`
	height: var(--size-8);
	border-bottom: calc(var(--size-1) / 2) solid var(--theme-wireframe-1);
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

var HeaderTitleStyle = ui.MustParseStyle(`
	font-size: var(--font-size-3);
	font-weight: var(--font-weight-6);

	margin-right: var(--size-2);

	& a {
			color: var(--theme-foreground-0);
			text-decoration: none;
	}
`)

func HeaderTitle(ctx ui.Context, opts ...g.Node) g.Node {
	return h.H1(ctx.Class(HeaderTitleStyle), h.A(append(g.Group{hx.Boost("true")}, opts...)...))
}

var HeaderLinkStyle = ui.MustParseStyle(`
		font-size: var(--font-size-2);
		color: var(--theme-foreground-0);
		text-decoration: underline;
		
		padding-left: var(--size-3);
`)

func HeaderLink(ctx ui.Context, opts ...g.Node) g.Node {
	return h.A(c.JoinAttrs("class", g.Group(opts), ctx.Class(HeaderLinkStyle)))
}
