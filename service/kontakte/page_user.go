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
	"github.com/teapotovh/teapot/lib/ui/components"
	"github.com/teapotovh/teapot/lib/webhandler"
)

var ErrFetchUser = errors.New("error while fetching user information")

const InvalidGroupDN = "<invalid>"

const (
	firstNameID = "firstname"
	lastNameID  = "lastname"
	mailID      = "mail"
	homeID      = "home"
	uidID       = "uid"
	gidID       = "gid"
)

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

var UserHeaderStyle = ui.MustParseStyle(`
	margin: var(--size-3) 0;
`)

var UserDNStyle = ui.MustParseStyle(`
  display: flex;
  flex-direction: column;
	margin-bottom: var(--size-3);

	& div {
	  display: flex;
		flex-direction: row;
		align-items: center;

		& h3 {
			font-weight: var(--font-weight-4);
		}
	}

	& span {
		font-size: var(--font-size-1);
		color: var(--theme-wireframe-1);
		user-select: none;
	}
`)

var UserAdminStyle = ui.MustParseStyle(`
  border: var(--size-1) solid var(--theme-error-0);
	font-size: var(--font-size-1);
  padding: var(--size-2);
  margin: 0 var(--size-3);

  display: flex;
  align-items: center;
  height: 0%;
`)

var UserFormStyle = ui.MustParseStyle(`
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

var UserButtonGroupStyle = ui.MustParseStyle(`
	margin: var(--size-3) 0 !important;
`)

var UserChangePasswordStyle = ui.MustParseStyle(`
	background: var(--theme-error-0);
	color: var(--theme-background-9);
	text-decoration: none;

	&:focus {
		outline-color: var(--theme-error-1) !important;
	}
`)

// TODO: remove Class for style
func (u user) Render(ctx ui.Context) g.Node {
	return g.Group{
		h.H2(ctx.Class(UserHeaderStyle), g.Text("Overview")),
		h.Section(
			dn(ctx, u.user.Username, u.user.DN, u.user.Admin),

			h.Form(ctx.Class(UserFormStyle),
				components.ValueInput(ctx, firstNameID, "text", "First Name", u.user.Firstname),
				components.ValueInput(ctx, lastNameID, "text", "Last Name", u.user.Lastname),
				components.ValueInput(ctx, mailID, "text", "Mail", u.user.Mail),
				components.ValueInput(ctx, homeID, "text", "Unix Home", u.user.Home),
				components.ValueInput(ctx, uidID, "text", "Unix UID", strconv.Itoa(u.user.UID)),
				components.ValueInput(ctx, gidID, "text", "Unix GID", strconv.Itoa(u.user.GID)),

				h.Div(ctx.Class(UserButtonGroupStyle),
					h.A(ctx.Class(components.ButtonStyle, UserChangePasswordStyle),
						hx.Boost("true"),
						h.Href(PathPasswd(u.user.Username)),
						g.Text("Change Password"),
					),
					components.Button(ctx, h.Type("submit"), g.Text("Update")),
				),
			),
		),

		components.HorizontalLine(ctx),

		h.H2(ctx.Class(UserHeaderStyle), g.Text("Groups")),
		h.Section(h.Class("groups"), g.Map(u.user.Groups, func(access string) g.Node {
			return group(ctx, access)
		})),

		components.HorizontalLine(ctx),

		h.H2(ctx.Class(UserHeaderStyle), g.Text("Accesses")),
		h.Section(h.Class("accesses"), g.Map(u.user.Accesses, func(access string) g.Node {
			return group(ctx, access)
		})),
	}
}

func dn(ctx ui.Context, name, dn string, admin bool) g.Node {
	return h.Div(ctx.Class(UserDNStyle),
		h.Div(
			h.H3(g.Text(name)),
			g.If(admin, h.Div(ctx.Class(UserAdminStyle), g.Text("admin"))),
		),
		h.Span(g.Text(dn)),
	)
}

func group(ctx ui.Context, access string) g.Node {
	first := strings.Split(access, ",")[0]

	var name string
	if n, err := fmt.Sscanf(first, "cn=%s", &name); err != nil || n < 1 {
		name = InvalidGroupDN
	}

	return dn(ctx, name, access, false)
}
