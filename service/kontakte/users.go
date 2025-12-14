package kontakte

import (
	"errors"
	"fmt"
	"net/http"

	g "maragu.dev/gomponents"
	hx "maragu.dev/gomponents-htmx"
	h "maragu.dev/gomponents/html"

	"github.com/teapotovh/teapot/lib/ldap"
)

var ErrListUsers = errors.New("error while listing users")

func (srv *Server) users(users []*ldap.User) g.Node {
	return h.Section(h.Class("users"),
		h.H2(g.Text("Users")),
		g.Map(users, func(user *ldap.User) g.Node {
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

func (srv *Server) HandleUsersGet(w http.ResponseWriter, r *http.Request) (g.Node, error) {
	client, err := srv.factory.NewClient(r.Context())
	if err != nil {
		err = ErrorWithStatus(
			fmt.Errorf("error while constructing LDAP client: %w", err),
			http.StatusInternalServerError,
		)

		return ErrorPage(r, ErrLDAP), err
	}
	defer client.Close()

	users, err := client.Users()
	if err != nil {
		err = ErrorWithStatus(
			fmt.Errorf("error while fetching user from LDAP: %w", err),
			http.StatusInternalServerError,
		)

		return ErrorPage(r, ErrListUsers), err
	}

	w.WriteHeader(http.StatusOK)

	return Page(r, "Users", srv.users(users)), nil
}
