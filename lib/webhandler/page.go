package httphandler

import "github.com/teapotovh/teapot/lib/ui"

// Lang is the language used in all pages. This is hardcoded to English for
// convenience.
const Lang = "en"

type page interface {
	Title() string
	Language() string
	Description() string
}

type Page struct {
	ui.Component

	title       string
	description string
}

func NewPage(title, description string, component ui.Component) Page {
	return Page{Component: component, title: title, description: description}
}

// Ensure Page implements ui.Component.
var _ ui.Component = Page{}

// Ensure Page implements page.
var _ page = Page{}

func (p Page) Title() string       { return p.title }
func (p Page) Language() string    { return Lang }
func (p Page) Description() string { return p.description }
