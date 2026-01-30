package kontakte

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	g "maragu.dev/gomponents"
	hx "maragu.dev/gomponents-htmx"
	h "maragu.dev/gomponents/html"

	"github.com/teapotovh/teapot/lib/ldap"
	"github.com/teapotovh/teapot/lib/pagetitle"
	"github.com/teapotovh/teapot/lib/ui"
	"github.com/teapotovh/teapot/lib/webhandler"
	"github.com/teapotovh/teapot/service/kontakte/components"
)

var ErrFetchUser = errors.New("error while fetching user information")

const InvalidGroupDN = "<invalid>"

func (k *Kontakte) User(w http.ResponseWriter, r *http.Request) (ui.Component, error) {
	switch r.Method {
	case http.MethodGet:
		username := r.PathValue("username")

		client, err := k.factory.NewClient(r.Context())
		if err != nil {
			return nil, webhandler.NewInternalError(err, nil)
		}
		defer client.Close()

		usr, err := client.User(username)
		if err != nil {
			return nil, webhandler.NewInternalError(fmt.Errorf("error while fetching user %q from LDAP: %w", username, err), nil)
		}

		component := user{user: usr}

		return webhandler.NewPage(
			pagetitle.Title(username+"'s profile", App),
			"View the details of the user: "+username+" as stored in LDAP",
			component,
		), nil
	}

	return nil, fmt.Errorf("invalid method %q: %w", r.Method, webhandler.ErrBadRequest)
}

type user struct {
	user *ldap.User
}

// TODO: remove Class for style
func (u user) Render(ctx ui.Context) g.Node {
	return h.Section(h.Class("user"),
		h.H2(g.Text("Overview")),
		h.Section(h.Class("overview"),

			h.Div(h.Class("dn"),
				h.Div(
					h.H3(g.Text(u.user.Username)),
					g.If(u.user.Admin, h.Div(h.Class("admin"), g.Text("admin"))),
				),
				h.Span(g.Text(u.user.DN)),
			),

			h.Form(
				components.Field("First Name", u.user.Firstname, true),
				components.Field("Last Name", u.user.Lastname, true),
				components.Field("Mail", u.user.Mail, false),
				components.Field("Unix Home", u.user.Home, false),
				components.Field("Unix UID", strconv.Itoa(u.user.UID), false),
				components.Field("Unix GID", strconv.Itoa(u.user.GID), false),

				h.Div(h.Class("button-container"),
					h.A(hx.Boost("true"), h.Href(PathPasswd(u.user.Username)),
						h.Class("button"), g.Text("Change password"),
					),
					components.Button(h.Type("submit"), g.Text("Update")),
				),
			),
		),

		h.H2(g.Text("Groups")),
		h.Section(h.Class("groups"),
			g.Map(u.user.Groups, func(group string) g.Node {
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
			g.Map(u.user.Accesses, func(access string) g.Node {
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
