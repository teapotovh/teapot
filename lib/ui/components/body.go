package components

import (
	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"

	"github.com/teapotovh/teapot/lib/ui"
)

var BodyStyle = ui.MustParseStyle(`
	padding-top: var(--size-1);
	padding-bottom: var(--size-1);
	min-height: calc(100vh - var(--size-8));

	@media (min-width: 1024px) {
		padding-top: var(--size-2);
		padding-bottom: var(--size-2);
	}

	@media (min-width: 1440px) {
		padding-top: var(--size-4);
		padding-bottom: var(--size-4);
	}
`)

func Body(ctx ui.Context, opts ...g.Node) g.Node {
	return Grid(ctx, h.Main,
		Clamp(ctx, h.Section, ctx.Class(WireframeStyle, BodyStyle), g.Group(opts)),
	)
}
