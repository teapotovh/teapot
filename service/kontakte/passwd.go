package kontakte

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/kataras/muxie"
	g "maragu.dev/gomponents"
	hx "maragu.dev/gomponents-htmx"
	h "maragu.dev/gomponents/html"

	"github.com/teapotovh/teapot/service/kontakte/components"
)

var (
	ErrNoPasswdPermission  = errors.New("not authorized to change password for user")
	ErrMissingPasswords    = errors.New("missing passwords")
	ErrMismatchedPasswords = errors.New("passwords don't match")
	ErrPasswd              = errors.New("error while changing password")
)

func passwd(r *http.Request, username string) g.Node {
	auth := getAuth(r.Context())
	// If an administrator is performing the password change on the behalf of
	// another user.
	adminChange := auth != nil && auth.Admin && auth.Subject != username

	return h.Section(h.Class("passwd"),
		h.H2(
			g.Iff(adminChange, func() g.Node {
				return g.Textf("Change %s's password", username)
			}),
			g.Iff(!adminChange, func() g.Node {
				return g.Text("Change your password")
			}),
		),
		h.Form(
			hx.Ext("response-targets"),
			hx.Post(PathPasswd(username)),
			hx.Swap("innerHTML"),
			g.Attr("hx-target-error", "#error-container"),

			components.Input("password", "password", "New password"),
			components.Input("repeat-password", "password", "Repeat new password"),
			h.Div(h.Class("button-container"),
				components.Button(h.Type("submit"), g.Text("Change password")),
			),

			h.Div(h.ID("error-container")),
		),
	)
}

func (srv *Server) HandlePasswdGet(w http.ResponseWriter, r *http.Request) (g.Node, error) {
	auth := getAuth(r.Context())
	username := muxie.GetParam(w, "username")

	if auth.Subject != username && !auth.Admin {
		err := ErrorWithStatus(fmt.Errorf("invalid passwd request: %w", ErrNoPasswdPermission), http.StatusUnauthorized)
		return ErrorDialog(ErrNoPasswdPermission), err
	}

	w.WriteHeader(http.StatusOK)
	return Page(r, "Passwd", passwd(r, username)), nil
}

func (srv *Server) HandlePasswdPost(w http.ResponseWriter, r *http.Request) (g.Node, error) {
	password := r.FormValue("password")
	repeatPassword := r.FormValue("repeat-password")

	if password == "" || repeatPassword == "" {
		err := ErrorWithStatus(fmt.Errorf("invalid passwd request: %w", ErrMissingCredentials), http.StatusBadRequest)
		return ErrorDialog(ErrMissingCredentials), err
	}

	if password != repeatPassword {
		err := ErrorWithStatus(fmt.Errorf("invalid passwd request: %w", ErrMismatchedPasswords), http.StatusBadRequest)
		return ErrorDialog(ErrMismatchedPasswords), err
	}

	// NOTE: this code could be de-duplicated from above with a middleware
	auth := getAuth(r.Context())
	username := muxie.GetParam(w, "username")

	if auth.Subject != username && !auth.Admin {
		err := ErrorWithStatus(fmt.Errorf("invalid passwd request: %w", ErrNoPasswdPermission), http.StatusUnauthorized)
		return ErrorDialog(ErrNoPasswdPermission), err
	}

	client, err := srv.factory.NewClient(r.Context())
	if err != nil {
		err = ErrorWithStatus(
			fmt.Errorf("error while constructing LDAP client: %w", err),
			http.StatusInternalServerError,
		)
		return ErrorDialog(ErrLDAP), err
	}
	defer client.Close()

	if err := client.Passwd(username, password); err != nil {
		err = ErrorWithStatus(fmt.Errorf("error while performing LDAP passwd: %w", err), http.StatusInternalServerError)
		return ErrorDialog(ErrPasswd), err
	}

	w.WriteHeader(http.StatusOK)
	return SuccessDialog(
		h.Span(g.Text("Password successfully updated! "),
			h.A(hx.Boost("true"), h.Href(PathUser(username)), g.Text("Go back")),
		),
	), nil
}
