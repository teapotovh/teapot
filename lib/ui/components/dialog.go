package components

import (
	g "maragu.dev/gomponents"
	hx "maragu.dev/gomponents-htmx"
	h "maragu.dev/gomponents/html"

	"github.com/teapotovh/teapot/lib/ui"
)

const (
	DialogID        = "dialog"
	DialogContentID = "content"
)

var DialogStyle = ui.MustParseStyle(`
	padding: var(--size-4);
	padding-top: var(--size-3);
	border: calc(var(--size-1) / 2) solid var(--theme-wireframe-1);
	background: var(--theme-background-2);

	& .close {
	  width: 100%;
	  display: flex;
	  flex-direction: row;
	  justify-content: flex-end;
		margin-bottom: var(--size-3);

		& button {
		  padding: var(--size-2);
			line-height: calc(var(--size-3) / 2 + var(--size-1));
	  }
	}
`)

func Dialog(ctx ui.Context) g.Node {
	return g.Group{
		h.Dialog(
			ctx.Class(DialogStyle),
			h.ID(DialogID),
			hx.On("keydown", "if (event.key === 'Escape' || event.key === 'Esc') { disposeElements.call(this); };"),
			hx.On(":after-swap", "this.showModal();"),

			h.Form(
				h.Class("close"),
				h.Method("dialog"),
				hx.On("submit", "disposeElements.call(this);"),
				Button(ctx, g.Text("×")),
			),

			h.Div(h.ID(DialogContentID)),
		),
		h.Script(g.Raw(`
	  function disposeElements() {
	    return this.closest("dialog").querySelector("#` + DialogContentID + `")?.childNodes.forEach(child => child.remove())
	  }
		`)),
	}
}
