package kontakte

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/kataras/muxie"
	g "maragu.dev/gomponents"
	hx "maragu.dev/gomponents-htmx"
	h "maragu.dev/gomponents/html"

	"github.com/teapotovh/teapot/lib/ldap"
	"github.com/teapotovh/teapot/service/kontakte/components"
)

var ErrFetchUser = errors.New("error while fetching user information")

const InvalidGroupDN = "<invalid>"

func (srv *Server) user(user *ldap.User) g.Node {
	return h.Section(h.Class("user"),
		h.H2(g.Text("Overview")),
		h.Section(h.Class("overview"),

			h.Div(h.Class("dn"),
				h.Div(
					h.H3(g.Text(user.Username)),
					g.If(user.Admin, h.Div(h.Class("admin"), g.Text("admin"))),
				),
				h.Span(g.Text(user.DN)),
			),

			h.Form(
				components.Field("First Name", user.Firstname, true),
				components.Field("Last Name", user.Lastname, true),
				components.Field("Mail", user.Mail, false),
				components.Field("Unix Home", user.Home, false),
				components.Field("Unix UID", strconv.Itoa(user.UID), false),
				components.Field("Unix GID", strconv.Itoa(user.GID), false),

				h.Div(h.Class("button-container"),
					h.A(hx.Boost("true"), h.Href(PathPasswd(user.Username)),
						h.Class("button"), g.Text("Change password"),
					),
					components.Button(h.Type("submit"), g.Text("Update")),
				),
			),
		),

		h.H2(g.Text("Groups")),
		h.Section(h.Class("groups"),
			g.Map(user.Groups, func(group string) g.Node {
				first := strings.Split(group, ",")[0]

				var name string
				if n, err := fmt.Sscanf(first, "cn=%s", &name); err != nil || n < 1 {
					name = InvalidGroupDN
				}

				return h.Div(h.Class("group"),
					h.H3(g.Text(name)),
					h.Span(g.Text(group)),
				)
			}),
		),

		h.H2(g.Text("Accesses")),
		h.Section(h.Class("accesses"),
			g.Map(user.Accesses, func(access string) g.Node {
				first := strings.Split(access, ",")[0]

				var name string
				if n, err := fmt.Sscanf(first, "cn=%s", &name); err != nil || n < 1 {
					name = InvalidGroupDN
				}

				return h.Div(h.Class("access"),
					h.H3(g.Text(name)),
					h.Span(g.Text(access)),
				)
			}),
		),
	)
}

func (srv *Server) HandleUserGet(w http.ResponseWriter, r *http.Request) (g.Node, error) {
	username := muxie.GetParam(w, "username")

	client, err := srv.factory.NewClient(r.Context())
	if err != nil {
		err = ErrorWithStatus(
			fmt.Errorf("error while constructing LDAP client: %w", err),
			http.StatusInternalServerError,
		)

		return ErrorPage(r, ErrLDAP), err
	}
	defer client.Close()

	u, err := client.User(username)
	if err != nil {
		err = ErrorWithStatus(
			fmt.Errorf("error while fetching user from LDAP: %w", err),
			http.StatusInternalServerError,
		)

		return ErrorPage(r, ErrFetchUser), err
	}

	w.WriteHeader(http.StatusOK)

	return Page(r, "User", srv.user(u)), nil
}
