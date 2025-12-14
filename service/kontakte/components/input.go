package components

import (
	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"
)

func Input(id, typ, label string, opts ...g.Node) g.Node {
	return h.Div(h.Class("input"),
		h.Label(h.For(id), g.Text(label)),
		h.Input(append([]g.Node{h.Type(typ), h.ID(id), h.Name(id)}, opts...)...),
	)
}
