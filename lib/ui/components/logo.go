package components

import (
	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"

	"github.com/teapotovh/teapot/lib/ui"
)

var LogoStyle = ui.MustParseStyle(`
	height: var(--size-7);
	width: var(--size-7);
	background: var(--theme-brand0);
`)

func Logo(ctx ui.Context) g.Node {
	return h.Div(ctx.Class(LogoStyle))
}
