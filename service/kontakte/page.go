package kontakte

import (
	"net/http"

	g "maragu.dev/gomponents"
	hx "maragu.dev/gomponents-htmx"
	hh "maragu.dev/gomponents/components"
	h "maragu.dev/gomponents/html"
)

func Page(r *http.Request, title string, body g.Node) g.Node {
	auth := getAuth(r.Context())

	return hh.HTML5(hh.HTML5Props{
		Title: title + " - kontakte",

		Head: []g.Node{
			h.Script(h.Src("https://unpkg.com/htmx.org")),
			h.Script(h.Src("https://unpkg.com/htmx-ext-response-targets")),
			h.Link(h.Rel("stylesheet"), h.Href("https://unpkg.com/tailwind-normalize")),
			h.Link(h.Rel("stylesheet"), h.Href(PathStyle)),
		},

		Body: []g.Node{
			h.Nav(h.Class("header"), hx.Boost("true"),
				h.Div(
					h.A(h.Class("logo"), h.Href(PathIndex), g.Text("Teapot")),

					g.If(auth != nil && auth.Admin, h.A(h.Class("item"), h.Href(PathUsers), g.Text("Users"))),
					g.If(auth != nil && auth.Admin, h.A(h.Class("item"), h.Href(PathGroups), g.Text("Groups"))),
				),

				g.Iff(auth != nil, func() g.Node {
					return h.Span(
						g.Text("Hi, "),
						h.A(h.Href(PathUser(auth.Subject)), g.Text(auth.Subject)),
						g.Text("! "),
						h.A(h.Href(PathLogout), g.Text("Logout")),
					)
				}),
			),
			h.Main(h.Class("layout"),
				body,
			),
		},
	})
}
