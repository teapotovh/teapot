package httpauth

import (
	"net/http"
	"time"

	"github.com/teapotovh/teapot/lib/ldap"
)

const (
	authContextKey = "auth"
)

func GetAuth(r *http.Request) *Auth {
	val := r.Context().Value(authContextKey)
	if val == nil {
		return nil
	}
	return val.(*Auth)
}

// MustGetAuth is unsafe and may panic.
// Calls GetAuth and dereferences the pointer (which may be nil).
// Assumes authentication is required.
// Only use behind endpoints secured with (Basic|JWT)Auth.Required() middleware.
func MustGetAuth(r *http.Request) Auth {
	return *GetAuth(r)
}

type Auth struct {
	ExpiresAt *time.Time
	Username  string
	Admin     bool
}

func authFromUser(user *ldap.User, expiresAt *time.Time) Auth {
	return Auth{
		ExpiresAt: expiresAt,
		Username:  user.Username,
		Admin:     user.Admin,
	}
}
