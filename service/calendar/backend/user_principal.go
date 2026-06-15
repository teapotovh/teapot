package backend

import (
	"context"
	"errors"

	"github.com/teapotovh/teapot/lib/httpauth"
	"github.com/teapotovh/teapot/lib/webdav"
)

var ErrMissingAuthentication = errors.New("missing authentication")

type userPrincipal struct{}

func (u *userPrincipal) CurrentUserPrincipal(ctx context.Context) (string, error) {
	auth := httpauth.GetAuthContext(ctx)
	if auth == nil {
		return "", ErrMissingAuthentication
	}

	return "/" + auth.Username, nil
}

// Ensure userPrincipal implements webdav.UserPrincipalBackend.
var _ webdav.UserPrincipalBackend = &userPrincipal{}
