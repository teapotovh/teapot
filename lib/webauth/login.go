package webauth

import (
	"errors"
	"fmt"
	"net/http"

	g "maragu.dev/gomponents"
	hx "maragu.dev/gomponents-htmx"
	h "maragu.dev/gomponents/html"

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
	case "POST":
		username := r.FormValue("username")
		password := r.FormValue("password")
		if username == "" || password == "" {
			w.WriteHeader(http.StatusUnauthorized)
			return loginError{err: ErrMissingCredentials}, nil
		}

		cookie, err := wa.auth.Authenticate(r.Context(), username, password)
		if err != nil {
			if errors.Is(err, ErrInvalidCredentials) {
				w.WriteHeader(http.StatusUnauthorized)
				return loginError{err: ErrInvalidCredentials}, nil
			}
			// TODO: figure out how to render this properly in webhandler
			// ideally it should be a box
			return nil, webhandler.NewInternalError(err, nil)
		}

		http.SetCookie(w, cookie)
		return nil, webhandler.NewRedirectError(wa.returnPath, http.StatusFound)

	case "GET":
		if GetAuth(r) != nil {
			return nil, webhandler.NewRedirectError(wa.returnPath, http.StatusFound)
		}

		component := login{path: wa.loginPath}
		return webhandler.NewPage("login", "login into files", component), nil
	}

	return nil, fmt.Errorf("invalid method %q: %w", r.Method, webhandler.ErrBadRequest)
}

type loginError struct {
	err error
}

func (le loginError) Render(ctx ui.Context) g.Node {
	return components.ErrorDialog(ctx, le.err)
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

const errorContainerID = "error-container"

type login struct {
	path string
}

func (l login) Render(ctx ui.Context) g.Node {
	return h.Div(ctx.Class(LoginBoxStyle),
		h.H2(g.Text("Login")),
		h.Form(
			hx.Ext("response-targets"),
			hx.Post(l.path),
			hx.Swap("innerHTML"),
			g.Attr("hx-target-error", "#"+errorContainerID),

			components.Input(ctx, "username", "text", "Username"),
			components.Input(ctx, "password", "password", "Password"),
			h.Div(h.Class("buttons"),
				components.Button(ctx, h.Type("submit"), g.Text("Login")),
			),

			h.Div(h.ID(errorContainerID), h.Class("error")),
		),
	)
}

// Ensure login implements ui.Component.
var _ ui.Component = login{}
