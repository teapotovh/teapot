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
	"github.com/teapotovh/teapot/lib/webauth"
	"github.com/teapotovh/teapot/lib/webhandler"
)

func (k *Kontakte) Groups(w http.ResponseWriter, r *http.Request) (ui.Component, error) {
	auth := webauth.GetAuth(r)
	if auth == nil || !auth.Admin {
		return nil, webhandler.NewRedirectError(PathIndex, http.StatusFound)
	}

	switch r.Method {
	case http.MethodGet:
		client, err := k.ldapFactory.NewClient(r.Context())
		if err != nil {
			return nil, webhandler.NewInternalError(err, nil)
		}
		defer client.Close()

		usrs, err := client.Groups(r.Context())
		if err != nil {
			return nil, webhandler.NewInternalError(fmt.Errorf("error while enumerating groups from LDAP: %w", err), nil)
		}

		component := groups{
			groups: usrs,
		}

		return webhandler.NewPage(
			pagetitle.Title("Browse all groups", App),
			"Browse all groups stored in the LDAP directory",
			component,
		), nil
	}

	return nil, fmt.Errorf("invalid method %q: %w", r.Method, webhandler.ErrBadRequest)
}

type groups struct {
	groups []*ldap.Group
}

var GroupsLineStyle = ui.MustParseStyle(`
	border: 1px solid var(--theme-wireframe-0);
  margin: 1em 0;
  padding: .5em;

  display: flex;
  flex-direction: row;
  justify-content: space-between;
  align-items: center;

  text-decoration: none;
`)

func (u groups) Render(ctx ui.Context) g.Node {
	return h.Section(
		h.H2(ctx.Class(HeaderStyle), g.Text("Groups")),
		g.Map(u.groups, func(group *ldap.Group) g.Node {
			return h.Div(ctx.Class(GroupsLineStyle),
				h.A(
					hx.Boost("true"),
					h.Href(PathGroup(group.Groupname)),
					dn(ctx, group.Groupname, group.DN, false),
				),
			)
		}),
	)
}
