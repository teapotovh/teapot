package components

import (
	g "maragu.dev/gomponents"
	c "maragu.dev/gomponents/components"
	h "maragu.dev/gomponents/html"

	"github.com/teapotovh/teapot/lib/ui"
)

var DialogStyle = ui.MustParseStyle(`
  padding: var(--size-2) var(--size-1);
`)

func Dialog(ctx ui.Context, children ...g.Node) g.Node {
	return h.Div(c.JoinAttrs("class", g.Group(children), ctx.Class(DialogStyle)))
}

var ErrorDialogStyle = ui.MustParseStyle(`
  background: var(--theme-error-0);
  border: var(--size-1) solid var(--theme-error-1);
  color: var(--theme-background-3);
`)

func ErrorDialog(ctx ui.Context, err error) g.Node {
	return Dialog(ctx, c.JoinAttrs("class", g.Group{g.Textf("Error: %s", err)}, ctx.Class(ErrorDialogStyle)))
}
