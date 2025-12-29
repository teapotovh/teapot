package components

import (
	g "maragu.dev/gomponents"
	c "maragu.dev/gomponents/components"
	h "maragu.dev/gomponents/html"

	"github.com/teapotovh/teapot/lib/ui"
)

var InputStyle = ui.MustParseStyle(`
  display: flex;
  flex-direction: column;

	& label {
		margin-bottom: var(--size-1);
		cursor: auto;
	}

	& input {
	  font-size: var(--font-size-4);
		border: calc(var(--size-1) / 2) solid var(--theme-wireframe-1);
		background: var(--theme-background-2);
		padding: calc(var(--size-1) / 2) var(--size-2);
		transition: outline 0.1s var(--ease-in-out-1);
	}

  & input:focus {
		outline-offset: 0;
		outline: var(--size-1) solid var(--theme-brand-1);
	}
`)

func Input(ctx ui.Context, id, typ, label string, opts ...g.Node) g.Node {
	return h.Div(c.JoinAttrs("class", append(g.Group{ctx.Class(InputStyle)}, opts...)),
		h.Label(h.For(id), g.Text(label)),
		h.Input(h.Type(typ), h.ID(id), h.Name(id)),
	)
}

var FileInputStyle = ui.MustParseStyle(`
	& label {
		font-size: var(--font-size-4);
		border: calc(var(--size-1) / 2) solid var(--theme-wireframe-1);
		background: var(--theme-background-2);
		padding: calc(var(--size-1) / 2) var(--size-2);
	}

  & label:focus {
		outline-offset: 0;
		outline: var(--size-1) solid var(--theme-brand-1);
	}

	& input {
		display: none;
	}

`)

func FileInput(ctx ui.Context, id, label string, opts ...g.Node) g.Node {
	return h.Div(ctx.Class(FileInputStyle),
		h.Label(h.For(id), g.Text(label), h.TabIndex("0")),
		h.Input(append(g.Group{h.Type("file"), h.ID(id), h.Name(id)}, opts...)...),
	)
}
