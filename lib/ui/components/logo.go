package components

import (
	_ "embed"

	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"

	"github.com/teapotovh/teapot/lib/ui"
)

//go:embed logo.svg
var logoSVG []byte

var LogoStyle = ui.MustParseStyle(`
	height: var(--size-7);
	width: var(--size-7);
`)

func Logo(ctx ui.Context) g.Node {
	return h.Div(ctx.Class(LogoStyle), g.Raw(string(logoSVG)))
}
