package httpauth

import (
	"context"
	"net/http"
	"time"

	"github.com/teapotovh/teapot/lib/ldap"
)

type contextKey string

const (
	authContextKey contextKey = "auth"
)

func GetAuth(r *http.Request) *Auth {
	return GetAuthContext(r.Context())
}

func GetAuthContext(ctx context.Context) *Auth {
	val := ctx.Value(authContextKey)
	if val == nil {
		return nil
	}

	return val.(*Auth)
}

type Auth struct {
	ExpiresAt *time.Time
	Username  string
	Admin     bool
}

// MustGetAuth is unsafe and may panic.
// Calls GetAuth and dereferences the pointer (which may be nil).
// Assumes authentication is required.
// Only use behind endpoints secured with (Basic|JWT)Auth.Required() middleware.
func MustGetAuth(r *http.Request) Auth {
	return *GetAuth(r)
}

func MustGetAuthContext(ctx context.Context) Auth {
	return *GetAuthContext(ctx)
}

func authFromUser(user *ldap.User, expiresAt *time.Time) Auth {
	return Auth{
		ExpiresAt: expiresAt,
		Username:  user.Username,
		Admin:     user.Admin,
	}
}
