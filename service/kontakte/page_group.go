package kontakte

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"

	g "maragu.dev/gomponents"
	hx "maragu.dev/gomponents-htmx"
	h "maragu.dev/gomponents/html"

	"github.com/teapotovh/teapot/lib/ldap"
	"github.com/teapotovh/teapot/lib/pagetitle"
	"github.com/teapotovh/teapot/lib/ui"
	"github.com/teapotovh/teapot/lib/ui/components"
	"github.com/teapotovh/teapot/lib/webauth"
	"github.com/teapotovh/teapot/lib/webhandler"
)

var ErrFetchGroup = errors.New("error while fetching group information")

const (
	groupNameID = "firstname"
	groupGIDID  = "gid"
)

func (k *Kontakte) Group(w http.ResponseWriter, r *http.Request) (ui.Component, error) {
	auth := webauth.GetAuth(r)
	if auth == nil || !auth.Admin {
		return nil, webhandler.NewRedirectError(PathIndex, http.StatusFound)
	}

	groupname := r.PathValue("groupname")

	switch r.Method {
	case http.MethodGet:
		client, err := k.ldapFactory.NewClient(r.Context())
		if err != nil {
			return nil, webhandler.NewInternalError(err, nil)
		}
		defer client.Close()

		usr, err := client.Group(r.Context(), groupname)
		if err != nil {
			return nil, webhandler.NewInternalError(
				fmt.Errorf("error while fetching group %q from LDAP: %w", groupname, err),
				nil,
			)
		}

		component := group{group: usr}

		return webhandler.NewPage(
			pagetitle.Title(groupname+"'s profile", App),
			"View the details of the group: "+groupname+" as stored in LDAP",
			component,
		), nil
	}

	return nil, fmt.Errorf("invalid method %q: %w", r.Method, webhandler.ErrBadRequest)
}

type group struct {
	group *ldap.Group
}

var GroupFormStyle = ui.MustParseStyle(`
  display: flex;
  flex-direction: column;

	& div {
		margin-top: var(--size-3);

		display: flex;
		flex-direction: row;
		justify-content: space-between;

		& input {
			font-size: var(--font-size-3);
		}
	}
`)

var GroupButtonGroupStyle = ui.MustParseStyle(`
	margin: var(--size-3) 0 !important;
`)

func (gr group) Render(ctx ui.Context) g.Node {
	return g.Group{
		h.H2(ctx.Class(HeaderStyle), g.Text("Overview")),
		h.Section(
			dn(ctx, gr.group.Groupname, gr.group.DN, false),

			h.Form(ctx.Class(GroupFormStyle),
				components.ValueInput(ctx, groupNameID, "text", "Group Name", gr.group.Groupname, true),
				components.ValueInput(ctx, groupGIDID, "text", "Unix GID", strconv.Itoa(gr.group.GID), false),

				h.Div(ctx.Class(GroupButtonGroupStyle),
					components.Button(ctx, h.Disabled(), h.Type("submit"), g.Text("Update")),
				),
			),
		),

		components.HorizontalLine(ctx),

		h.H2(ctx.Class(HeaderStyle), g.Text("Memebers")),
		h.Section(g.Map(gr.group.Memebers, func(access string) g.Node {
			return userLink(ctx, access)
		})),
	}
}

func userLink(ctx ui.Context, user string) g.Node {
	name := nameFromDN(user)

	return h.A(
		hx.Boost("true"),
		h.Href(PathUser(name)),
		dn(ctx, name, user, false),
	)
}
