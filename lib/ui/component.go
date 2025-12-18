package ui

import (
	g "maragu.dev/gomponents"
)

type Component interface {
	Render(ctx Context) g.Node
}

type ComponentFunc func(ctx Context) g.Node

func (cf ComponentFunc) Render(ctx Context) g.Node {
	return cf(ctx)
}

// Ensure ComponentFunc implements Component.
var _ Component = ComponentFunc(func(ctx Context) g.Node { return nil })
