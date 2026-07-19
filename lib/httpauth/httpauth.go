package httpauth

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/teapotovh/teapot/lib/ldap"
	"github.com/teapotovh/teapot/lib/observability"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
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

func authenticate(ctx context.Context, factory *ldap.Factory, username, password string, logger *slog.Logger) (user *ldap.User, err error) {
	ctx, span := observability.TracerFromContext(ctx).Start(ctx, "BasicAuth.Middleware")
	defer observability.SpanEnd(span, err)

	client, err := factory.NewClient(ctx)
	if err != nil {
		logger.ErrorContext(ctx, "error while creating LDAP client", "err", err)
		return nil, err
	}
	defer client.Close()

	user, err = client.Authenticate(ctx, username, password)
	if err != nil {
		if errors.Is(err, ldap.ErrInvalidCredentials) {
			return nil, ErrInvalidCredentials
		}

		logger.ErrorContext(ctx, "unexpected error while authenticating", "username", username, "err", err)
		return nil, err
	}

	return user, nil
}
