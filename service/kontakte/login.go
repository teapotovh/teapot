package kontakte

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	g "maragu.dev/gomponents"
	hx "maragu.dev/gomponents-htmx"
	hxhttp "maragu.dev/gomponents-htmx/http"
	h "maragu.dev/gomponents/html"

	"github.com/teapotovh/teapot/service/kontakte/components"
)

var (
	ErrMissingCredentials = errors.New("missing credentials")
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrAuthToken          = errors.New("error while genrating authentication token")
)

func login() g.Node {
	return h.Section(h.Class("login"),
		h.H2(g.Text("Login")),
		h.Form(
			hx.Ext("response-targets"),
			hx.Post("/login"),
			hx.Swap("innerHTML"),
			g.Attr("hx-target-error", "#error-container"),

			components.Input("username", "text", "Username"),
			components.Input("password", "password", "Password"),
			h.Div(h.Class("button-container"),
				components.Button(h.Type("submit"), g.Text("Login")),
			),

			h.Div(h.ID("error-container")),
		),
	)
}

func (srv *Server) HandleLoginGet(w http.ResponseWriter, r *http.Request) (g.Node, error) {
	auth := getAuth(r.Context())
	if auth != nil {
		Redirect(w, r, PathUser(auth.Subject))
		return nil, nil
	}

	w.WriteHeader(http.StatusOK)
	return Page(r, "Login", login()), nil
}

func (srv *Server) HandleLoginPost(w http.ResponseWriter, r *http.Request) (g.Node, error) {
	username := r.FormValue("username")
	password := r.FormValue("password")

	if username == "" || password == "" {
		err := ErrorWithStatus(fmt.Errorf("invalid login request: %w", ErrMissingCredentials), http.StatusBadRequest)
		return ErrorDialog(ErrMissingCredentials), err
	}

	client, err := srv.factory.NewClient(r.Context())
	if err != nil {
		err = ErrorWithStatus(fmt.Errorf("error while constructing LDAP client: %w", err), http.StatusInternalServerError)
		return ErrorDialog(ErrLDAP), err
	}
	defer client.Close()

	user, err := client.Authenticate(username, password)
	if err != nil {
		err = ErrorWithStatus(fmt.Errorf("error while performing user bind: %w", err), http.StatusUnauthorized)
		return ErrorDialog(ErrInvalidCredentials), err
	}

	cookie, err := srv.authCookie(username, user.Admin)
	if err != nil {
		err = ErrorWithStatus(fmt.Errorf("error while generating authentication cookie: %w", err), http.StatusInternalServerError)
		return ErrorDialog(ErrAuthToken), err
	}

	http.SetCookie(w, cookie)
	redirectURL := r.URL.Query().Get("redirect")
	if redirectURL != "" {
		http.Redirect(w, r, redirectURL, http.StatusFound)
	} else {
		Redirect(w, r, PathUser(username))
	}

	return nil, nil
}

func (srv *Server) HandleLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:    authCookieName,
		Value:   "",
		Path:    PathIndex,
		Expires: time.Now(),
	})
	if hxhttp.IsRequest(r.Header) {
		// If the request was sent from HTMX, it will not follow multiple redirects,
		// so we want to skip the redirect to login from the index page.
		hxhttp.SetLocation(w.Header(), PathLogin)
	} else {
		http.Redirect(w, r, PathIndex, http.StatusFound)
	}
}
