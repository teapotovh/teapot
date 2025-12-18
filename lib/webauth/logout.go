package webauth

import (
	"fmt"
	"net/http"

	"github.com/teapotovh/teapot/lib/ui"
	"github.com/teapotovh/teapot/lib/webhandler"
)

func (wa *WebAuth) Logout(w http.ResponseWriter, r *http.Request) (ui.Component, error) {
	switch r.Method {
	case "GET":
		if GetAuth(r) != nil {
			cookie := wa.auth.DeAuthenticate()
			http.SetCookie(w, &cookie)
		}

		return nil, webhandler.NewRedirectError(wa.loginPath, http.StatusFound)
	}
	return nil, fmt.Errorf("invalid method %q: %w", r.Method, webhandler.ErrBadRequest)
}
