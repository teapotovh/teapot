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
	"github.com/teapotovh/teapot/lib/ui/components"
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

var UsersLineStyle = ui.MustParseStyle(`
	border: 1px solid var(--theme-wireframe-0);
  margin: 1em 0;
  padding: .5em;

  display: flex;
  flex-direction: row;
  justify-content: space-between;
  align-items: center;

  text-decoration: none;
`)

func (u users) Render(ctx ui.Context) g.Node {
	return h.Section(
		h.H2(ctx.Class(HeaderStyle), g.Text("Users")),
		g.Map(u.users, func(user *ldap.User) g.Node {
			return h.Div(ctx.Class(UsersLineStyle),
				h.A(
					hx.Boost("true"),
					h.Href(PathUser(user.Username)),
					dn(ctx, user.Username, user.DN, user.Admin),
				),

				h.A(ctx.Class(components.ButtonStyle),
					hx.Boost("true"),
					h.Href(PathPasswd(user.Username)),
					g.Text("Change Password"),
				),
			)
		}),
	)
}
