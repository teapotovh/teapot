package kontakte

import (
	"errors"
	"fmt"
	"net/http"

	g "maragu.dev/gomponents"
	hx "maragu.dev/gomponents-htmx"
	h "maragu.dev/gomponents/html"

	"github.com/teapotovh/teapot/lib/pagetitle"
	"github.com/teapotovh/teapot/lib/ui"
	"github.com/teapotovh/teapot/lib/ui/components"
	"github.com/teapotovh/teapot/lib/webauth"
	"github.com/teapotovh/teapot/lib/webhandler"
)

var (
	ErrMismatchedPasswords = errors.New("passwords don't match")
	ErrPasswd              = errors.New("error while changing password")
)

const (
	passwordID       = "password"
	repeatPasswordID = "repeat-password"
)

func (k *Kontakte) Passwd(w http.ResponseWriter, r *http.Request) (ui.Component, error) {
	auth := webauth.GetAuth(r)
	if auth == nil {
		return nil, webhandler.NewRedirectError(PathIndex, http.StatusFound)
	}

	username := r.PathValue("username")

	switch r.Method {
	case http.MethodGet:
		component := passwd{
			username:    username,
			adminChange: auth.Admin && auth.Username != username,
		}

		return webhandler.NewPage(
			pagetitle.Title("Change the password for "+username, App),
			"Change the password associated with the user "+username+" in the LDAP directory",
			component,
		), nil

	case http.MethodPost:
		password := r.FormValue(passwordID)
		repeatPassword := r.FormValue(repeatPasswordID)

		if password == "" || repeatPassword == "" {
			w.WriteHeader(http.StatusBadRequest)
			return dialogError{err: fmt.Errorf("invalid passwd request: %w", webauth.ErrMissingCredentials)}, nil
		}

		if password != repeatPassword {
			w.WriteHeader(http.StatusBadRequest)
			return dialogError{err: fmt.Errorf("invalid passwd request: %w", ErrMismatchedPasswords)}, nil
		}

		if auth.Username != username && !auth.Admin {
			w.WriteHeader(http.StatusUnauthorized)

			return dialogError{
				err: fmt.Errorf("not authorized to perform password update: %w", webhandler.ErrBadRequest),
			}, nil
		}

		client, err := k.factory.NewClient(r.Context())
		if err != nil {
			return nil, webhandler.NewInternalError(err, nil)
		}
		defer client.Close()

		if err := client.Passwd(username, password); err != nil {
			w.WriteHeader(http.StatusUnauthorized)
			return dialogError{err: fmt.Errorf("error while performing LDAP passwd: %w", err)}, nil
		}

		w.WriteHeader(http.StatusOK)

		return dialogSuccess{
			username: username,
			message:  "The password has been updated successfully.",
		}, nil
	}

	return nil, fmt.Errorf("invalid method %q: %w", r.Method, webhandler.ErrBadRequest)
}

type dialogError struct {
	err error
}

func (de dialogError) Render(ctx ui.Context) g.Node {
	return components.ErrorNotification(ctx, de.err)
}

type dialogSuccess struct {
	username string
	message  string
}

func (ds dialogSuccess) Render(ctx ui.Context) g.Node {
	component := h.Span(
		g.Text(ds.message+" "),
		h.A(hx.Boost("true"), h.Href(PathUser(ds.username)), g.Text("Go back")),
	)

	return components.SuccessNotification(ctx, component)
}

type passwd struct {
	username    string
	adminChange bool
}

var PasswdOtherUserStyle = ui.MustParseStyle(`
	font-weight: var(--font-weight-6);
`)

var PasswdInputStyle = ui.MustParseStyle(`
  margin: var(--size-4) 0;
`)

var PasswdButtonsStyle = ui.MustParseStyle(`
	display: flex;
  flex-direction: row;
  justify-content: flex-end;

	margin: var(--size-2) 0;
`)

var PasswdErrorStyle = ui.MustParseStyle(`
	margin: var(--size-3) 0;
`)

func (p passwd) Render(ctx ui.Context) g.Node {
	return g.Group{
		g.Iff(p.adminChange, func() g.Node {
			return components.WarningNotification(ctx, g.Group{
				h.Span(g.Text("You are changing ")),
				h.Span(ctx.Class(PasswdOtherUserStyle), g.Text(p.username)),
				h.Span(g.Text("'s pasword, not yours!")),
			})
		}),
		h.H2(ctx.Class(HeaderStyle), g.Text("Update password")),
		h.Form(
			hx.Ext("response-targets"),
			hx.Post(PathPasswd(p.username)),
			hx.Swap("innerHTML"),
			g.Attr("hx-target-error", "#error-container"),

			components.Input(ctx, passwordID, "password", "New password", ctx.Class(PasswdInputStyle)),
			components.Input(ctx, repeatPasswordID, "password", "Repeat new password", ctx.Class(PasswdInputStyle)),

			h.Div(ctx.Class(PasswdButtonsStyle),
				components.Button(ctx, h.Type("submit"), g.Text("Change password")),
			),

			h.Div(ctx.Class(PasswdErrorStyle), h.ID("error-container")),
		),
	}
}
