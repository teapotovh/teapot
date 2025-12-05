package components

import (
	g "maragu.dev/gomponents"
	c "maragu.dev/gomponents/components"
	h "maragu.dev/gomponents/html"

	"github.com/teapotovh/teapot/lib/ui"
)

var ButtonStyle = ui.MustParseStyle(`
	color: var(--theme-background0);
	background: var(--theme-brand0);
	padding: .25em .5em;
	transition: outline 0.1s ease-in-out, background 0.1s ease-in-out;
	cursor: pointer;

	&.secondary {
		color: var(--theme-brand1);
		background: var(--theme-foreground0);
	}
`)

func Button(ctx ui.Context, children ...g.Node) g.Node {
	return h.Button(c.JoinAttrs("class", g.Group(children), ctx.Class(ButtonStyle)))
}
