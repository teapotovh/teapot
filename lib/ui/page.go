package ui

import (
	g "maragu.dev/gomponents"
	c "maragu.dev/gomponents/components"
)

type PageOptions struct {
	Title   string
	Styles  []g.Node
	Scripts []g.Node
	Body    []g.Node
}

type Page interface {
	Render(opts PageOptions) g.Node
}

// Ensure DefaultPage implements Page.
var _ Page = DefaultPage{}

type DefaultPage struct{}

func (DefaultPage) Render(opts PageOptions) g.Node {
	return c.HTML5(c.HTML5Props{
		Title: opts.Title,
		Head:  append(opts.Styles, opts.Scripts...),
		Body:  opts.Body,
	})
}
