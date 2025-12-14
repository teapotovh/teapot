package kontakte

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/kataras/muxie"
	g "maragu.dev/gomponents"
)

const (
	authCookieName = "kontakte-auth"
	authDuration   = time.Hour * 48
)

var ErrInvalidAuthTokenType = errors.New("")

type Auth struct {
	jwt.RegisteredClaims
	Admin bool `json:"admin"`
}

func (srv *Server) authCookie(username string, admin bool) (*http.Cookie, error) {
	expiry := time.Now().Add(authDuration)

	claims := &Auth{
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expiry),
			Subject:   username,
		},
		Admin: admin,
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	ss, err := token.SignedString(srv.jwtSecret)
	if err != nil {
		return nil, fmt.Errorf("error while signing JWT: %w", err)
	}

	return &http.Cookie{
		Name:    authCookieName,
		Value:   ss,
		Path:    "/",
		Expires: expiry,
	}, nil
}

func (srv *Server) checkAuthCookie(r *http.Request) *Auth {
	cookie, err := r.Cookie(authCookieName)
	if err != nil {
		if !errors.Is(err, http.ErrNoCookie) {
			srv.logger.ErrorContext(r.Context(), "error while fetching authentication cookie", "err", err)
		}
		return nil
	}
	if cookie == nil {
		return nil
	}

	token, err := jwt.ParseWithClaims(cookie.Value, &Auth{}, func(token *jwt.Token) (any, error) {
		return srv.jwtSecret, nil
	})
	if err != nil {
		srv.logger.ErrorContext(r.Context(), "error while validating authentication cookie", "err", err)
		return nil
	} else if claims, ok := token.Claims.(*Auth); ok {
		return claims
	} else {
		srv.logger.ErrorContext(r.Context(), "validation token is of unexpected type", "err", err)
		return nil
	}
}

const authContextKey contextKey = "auth"

func getAuth(ctx context.Context) *Auth {
	return ctx.Value(authContextKey).(*Auth)
}

func (srv *Server) AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := srv.checkAuthCookie(r)
		r = r.WithContext(context.WithValue(r.Context(), authContextKey, auth))
		next.ServeHTTP(w, r)
	})
}

func AdminOrSelfMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := getAuth(r.Context())
		if auth == nil {
			Redirect(w, r, PathLogin)
			return
		}

		// get username, check with Subject or admin
		username := muxie.GetParam(w, "username")
		if auth.Admin || auth.Subject == username {
			next.ServeHTTP(w, r)
		} else {
			fn := func(w http.ResponseWriter, r *http.Request) (g.Node, error) {
				return Unauthorized(r)
			}
			Adapt(fn).ServeHTTP(w, r)
		}
	})
}
