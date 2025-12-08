package ui

import (
	"strings"

	"github.com/teapotovh/teapot/lib/ui/dependency"
	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"
)

type Context interface {
	Class(...*Style) g.Node
}

type unit struct{}

type context struct {
	renderer *Renderer

	styles       map[*Style]unit
	dependencies map[dependency.Dependency]unit
}

// Ensure *context implements Context
var _ Context = (*context)(nil)

func (c *context) register(style *Style) {
	c.styles[style] = unit{}
	for _, dep := range style.dependencies {
		c.dependencies[dep] = unit{}
	}
}

func (c *context) Class(styles ...*Style) g.Node {
	var ids = make([]string, len(styles))
	for _, style := range styles {
		if _, ok := c.styles[style]; !ok {
			c.register(style)
		}
		ids = append(ids, style.id)
	}

	return h.Class(strings.Join(ids, " "))
}
