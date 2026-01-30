package kontakte

import (
	"fmt"
	"net/http"

	g "maragu.dev/gomponents"
	hx "maragu.dev/gomponents-htmx"
	h "maragu.dev/gomponents/html"

	"github.com/teapotovh/teapot/lib/ldap"
	"github.com/teapotovh/teapot/lib/pagetitle"
	"github.com/teapotovh/teapot/lib/ui"
	"github.com/teapotovh/teapot/lib/webhandler"
)

func (k *Kontakte) Users(w http.ResponseWriter, r *http.Request) (ui.Component, error) {
	switch r.Method {
	case http.MethodGet:
		client, err := k.factory.NewClient(r.Context())
		if err != nil {
			return nil, webhandler.NewInternalError(err, nil)
		}
		defer client.Close()

		usrs, err := client.Users()
		if err != nil {
			return nil, webhandler.NewInternalError(fmt.Errorf("error while enumerating users from LDAP: %w", err), nil)
		}

		component := users{
			users: usrs,
		}

		return webhandler.NewPage(
			pagetitle.Title("Browse all users", App),
			"Browse all users stored in the LDAP directory",
			component,
		), nil
	}

	return nil, fmt.Errorf("invalid method %q: %w", r.Method, webhandler.ErrBadRequest)
}

type users struct {
	users []*ldap.User
}

// TODO: remove Class for style
func (u users) Render(ctx ui.Context) g.Node {
	return h.Section(h.Class("users"),
		h.H2(g.Text("Users")),
		g.Map(u.users, func(user *ldap.User) g.Node {
			return h.Div(h.Class("line"),
				h.A(hx.Boost("true"), h.Href(PathUser(user.Username)),
					h.Class("dn"),
					h.H4(g.Text(user.Username)),
					h.Span(g.Text(user.DN)),
				),

				h.A(hx.Boost("true"), h.Href(PathPasswd(user.Username)),
					h.Class("button"), g.Text("Change password"),
				),
			)
		}),
	)
}
