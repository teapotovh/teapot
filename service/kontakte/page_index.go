package kontakte

import (
	"net/http"

	"github.com/teapotovh/teapot/lib/ui"
	"github.com/teapotovh/teapot/lib/webauth"
	"github.com/teapotovh/teapot/lib/webhandler"
)

func (k *Kontakte) Index(w http.ResponseWriter, r *http.Request) (ui.Component, error) {
	if auth := webauth.GetAuth(r); auth != nil {
		return nil, webhandler.NewRedirectError(PathUser(auth.Username), http.StatusFound)
	}

	return nil, webhandler.NewRedirectError(PathLogin, http.StatusFound)
}
