package ui

import (
	"github.com/teapotovh/teapot/lib/ui/dependency"
	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"
)

type Context interface {
	Class(*Style) g.Node
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

func (c *context) Class(style *Style) g.Node {
	if _, ok := c.styles[style]; !ok {
		c.register(style)
	}

	return h.Class(style.id)
}
