package components

import (
	"strings"

	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"
)

func Field(name, value string, enabled bool, opts ...g.Node) g.Node {
	id := strings.ToLower(strings.Replace(name, " ", "-", -1))

	attrs := []g.Node{h.Type("text"), h.ID(id), h.Name(id), h.Value(value)}
	if !enabled {
		attrs = append(attrs, h.Disabled())
	}

	return h.Div(h.Class("field"),
		h.Label(h.For(id), g.Text(name)),
		h.Input(append(attrs, opts...)...),
	)
}
