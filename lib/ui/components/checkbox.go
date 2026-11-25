package components

import (
	g "maragu.dev/gomponents"
	c "maragu.dev/gomponents/components"
	h "maragu.dev/gomponents/html"

	"github.com/teapotovh/teapot/lib/ui"
)

var CheckboxStyle = ui.MustParseStyle(`
	appearance: none;
	padding: 0;
	position: relative;
	border: calc(var(--size-1) / 2) solid var(--theme-wireframe-1);
	background: var(--theme-background-2);

	&:checked {
	  background: var(--theme-brand-1);
	  border-color: var(--theme-brand-1);
	}

	&:checked::after {
	  content: "";
	  position: absolute;
	  top: 50%;
	  left: 50%;
	  width: var(--size-1);
	  height: var(--size-2);
	  border: solid var(--theme-background-0);
	  border-width: 0 calc(var(--size-1) / 2) calc(var(--size-1) / 2) 0;
	  transform: translate(-50%, -60%) rotate(45deg);
	}

	&:focus {
	  outline-offset: 0;
	  outline: var(--size-1) solid var(--theme-brand-1);
	}
`)

func Checkbox(ctx ui.Context, label string, opts ...g.Node) g.Node {
	attrs := append(g.Group{
		h.Type("checkbox"),
		g.Attr("aria-label", label),
	}, opts...)

	return h.Input(c.JoinAttrs("class", append(g.Group{ctx.Class(CheckboxStyle)}, attrs...)))
}
