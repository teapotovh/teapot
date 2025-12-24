package components

import (
	g "maragu.dev/gomponents"
	hx "maragu.dev/gomponents-htmx"
	c "maragu.dev/gomponents/components"
	h "maragu.dev/gomponents/html"

	"github.com/teapotovh/teapot/lib/ui"
)

var ButtonStyle = ui.MustParseStyle(`
	color: var(--theme-background-0);
	background: var(--theme-brand-1);
	padding: var(--size-1) var(--size-2);
	transition: outline 0.1s var(--ease-in-out-1), background 0.1s var(--ease-in-out-1);
	cursor: pointer;
		transition: outline 0.1s var(--ease-in-out-1);

	&:focus {
		outline-offset: 0;
		outline: var(--size-1) solid var(--theme-brand-0);
	}
`)

func Button(ctx ui.Context, children ...g.Node) g.Node {
	return h.Button(c.JoinAttrs("class", g.Group(children), ctx.Class(ButtonStyle)))
}

func DialogButton(ctx ui.Context, dialogURL string, children ...g.Node) g.Node {
	return h.Button(
		hx.Get(dialogURL),
		hx.Target("#"+DialogID+" #"+DialogContentID),
		hx.Swap("inner"),
		c.JoinAttrs("class", g.Group(children), ctx.Class(ButtonStyle)),
	)
}
