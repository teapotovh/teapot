package auth

import (
	"net/http"

	"github.com/teapotovh/teapot/lib/ui"
)

func (k *Auth) Redirect(w http.ResponseWriter, r *http.Request) (ui.Component, error) {
	return nil, nil
}
