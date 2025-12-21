package web

import (
	"net/http"

	"github.com/teapotovh/teapot/lib/ui"
	"github.com/teapotovh/teapot/lib/webauth"
	"github.com/teapotovh/teapot/lib/webhandler"
)

func (web *Web) Index(w http.ResponseWriter, r *http.Request) (ui.Component, error) {
	if webauth.GetAuth(r) != nil {
		return nil, webhandler.NewRedirectError(PathBrowse, http.StatusFound)
	}

	return nil, webhandler.NewRedirectError(PathLogin, http.StatusFound)
}
