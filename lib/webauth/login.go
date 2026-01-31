package webauth

import (
	"errors"
	"fmt"
	"net/http"

	g "maragu.dev/gomponents"
	hx "maragu.dev/gomponents-htmx"
	h "maragu.dev/gomponents/html"

	"github.com/teapotovh/teapot/lib/httpauth"
	"github.com/teapotovh/teapot/lib/pagetitle"
	"github.com/teapotovh/teapot/lib/ui"
	"github.com/teapotovh/teapot/lib/ui/components"
	"github.com/teapotovh/teapot/lib/webhandler"
)

var (
	ErrMissingCredentials = errors.New("missing credentials")
	ErrInvalidCredentials = errors.New("invalid credentials")
)

func (wa *WebAuth) Login(w http.ResponseWriter, r *http.Request) (ui.Component, error) {
	switch r.Method {
	case http.MethodPost:
		username := r.FormValue("username")

		password := r.FormValue("password")
		if username == "" || password == "" {
			w.WriteHeader(http.StatusUnauthorized)
			return loginError{err: ErrMissingCredentials}, nil
		}

		cookie, auth, err := wa.auth.Authenticate(r.Context(), username, password)
		if err != nil {
			if errors.Is(err, httpauth.ErrInvalidCredentials) {
				w.WriteHeader(http.StatusUnauthorized)
				return loginError{err: ErrInvalidCredentials}, nil
			}

			return nil, webhandler.NewInternalError(err, nil)
		}

		http.SetCookie(w, cookie)

		return nil, webhandler.NewRedirectError(wa.returnPath(*auth), http.StatusFound)

	case http.MethodGet:
		if auth := GetAuth(r); auth != nil {
			return nil, webhandler.NewRedirectError(wa.returnPath(*auth), http.StatusFound)
		}

		component := login{path: wa.loginPath}

		return webhandler.NewPage(
			pagetitle.Title("Login", wa.app),
			"Authenticate with your account to access "+wa.app,
			component,
		), nil
	}

	return nil, fmt.Errorf("invalid method %q: %w", r.Method, webhandler.ErrBadRequest)
}

type loginError struct {
	err error
}

func (le loginError) Render(ctx ui.Context) g.Node {
	return components.ErrorNotification(ctx, le.err)
}

// Ensure Login implements webhandler.WebHandlerFunc.
var _ webhandler.WebHandlerFunc = (*WebAuth)(nil).Login

var LoginBoxStyle = ui.MustParseStyle(`
	display: flex;
	flex-direction: column;
	justify-content: center;
	height: 100%;
	padding: 0 var(--size-5);
	font-size: var(--font-size-2);

	h2 {
		font-size: var(--font-size-5);
    font-weight: var(--font-weight-4);
	}

	@media (min-width: 1024px) {
		& {
			padding: 0 var(--size-9);
		}
	}
	@media (min-width: 1920px) {
		& {
			padding: 0 var(--size-13);
		}
	}

	& .buttons {
		display: flex;
		flex-direction: row;
		justify-content: flex-end;
	}

	& .error {
		margin: var(--size-3) 0;
	}
`)

var LoginInputStyle = ui.MustParseStyle(`
  margin: var(--size-7) 0;
`)

const errorContainerID = "error-container"

type login struct {
	path string
}

func (l login) Render(ctx ui.Context) g.Node {
	// TODO: add password recovery URL
	return h.Div(ctx.Class(LoginBoxStyle),
		h.H2(g.Text("Login")),
		h.Form(
			hx.Ext("response-targets"),
			hx.Post(l.path),
			hx.Swap("innerHTML"),
			g.Attr("hx-target-error", "#"+errorContainerID),

			components.Input(ctx, "username", "text", "Username", ctx.Class(LoginInputStyle)),
			components.Input(ctx, "password", "password", "Password", ctx.Class(LoginInputStyle)),
			h.Div(h.Class("buttons"),
				components.Button(ctx, h.Type("submit"), g.Text("Login")),
			),

			h.Div(h.ID(errorContainerID), h.Class("error")),
		),
	)
}

// Ensure login implements ui.Component.
var _ ui.Component = login{}
