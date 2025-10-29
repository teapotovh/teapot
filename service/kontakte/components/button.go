package components

import (
	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"
)

func Button(opts ...g.Node) g.Node {
	return h.Button(append([]g.Node{h.Class("button")}, opts...)...)
}
