package httpauth

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/teapotovh/teapot/lib/ldap"
)

const (
	cookieName = "teapot-auth"
)

type JWTAuth struct {
	logger *slog.Logger

	secret   string
	issuer   string
	duration time.Duration
	prefix   string

	factory *ldap.Factory
}

type JWTAuthConfig struct {
	// Secret is the JWT signing secret. When multiple instances of the
	// application are deployed, all instances must use the same secret.
	Secret string
	// Issuer is the name of the application (the issuer of the tokens)
	Issuser string
	// Duration is the duration of the JWT
	Duration time.Duration
	// Prefix is the HTTP prefix under which the web application is served
	Prefix string
}

func NewJWTAuth(factory *ldap.Factory, config JWTAuthConfig, logger *slog.Logger) *JWTAuth {
	return &JWTAuth{
		logger: logger,

		secret:   config.Secret,
		issuer:   config.Issuser,
		duration: config.Duration,
		prefix:   config.Prefix,

		factory: factory,
	}
}

func (ja *JWTAuth) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := ja.checkAuthCookie(r)
		r = r.WithContext(context.WithValue(r.Context(), authContextKey, auth))
		next.ServeHTTP(w, r)
	})
}

func (ja *JWTAuth) Authenticate(w http.ResponseWriter, username string, admin bool) error {
	cookie, err := ja.authCookie(username, admin)
	if err != nil {
		return fmt.Errorf("error while generating JWT cookie: %w", err)
	}

	http.SetCookie(w, cookie)

	return nil
}

func (ja *JWTAuth) DeAuthenticate(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:    cookieName,
		Value:   "",
		Path:    ja.prefix,
		Expires: time.Now(),
	})
}

type jwtAuth struct {
	jwt.RegisteredClaims

	Admin bool `json:"admin,omitempty"`
}

func (ja *JWTAuth) authCookie(username string, admin bool) (*http.Cookie, error) {
	now := time.Now()
	expiry := now.Add(ja.duration)

	claims := &jwtAuth{
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expiry),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			Issuer:    ja.issuer,
			Subject:   username,
		},
		Admin: admin,
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	ss, err := token.SignedString(ja.secret)
	if err != nil {
		return nil, fmt.Errorf("error while signing JWT for %q: %w", username, err)
	}

	return &http.Cookie{
		Name:    cookieName,
		Value:   ss,
		Path:    ja.prefix,
		Expires: expiry,
	}, nil
}

func (ja *JWTAuth) checkAuthCookie(r *http.Request) *jwtAuth {
	cookie, err := r.Cookie(cookieName)
	if err != nil {
		if !errors.Is(err, http.ErrNoCookie) {
			ja.logger.ErrorContext(r.Context(), "unexpected error while fetching authentication cookie", "err", err)
		}

		return nil
	}

	if cookie == nil {
		return nil
	}

	token, err := jwt.ParseWithClaims(cookie.Value, &jwtAuth{}, func(token *jwt.Token) (any, error) {
		return ja.secret, nil
	})
	if err != nil {
		ja.logger.ErrorContext(r.Context(), "error while validating authentication cookie", "err", err)
		return nil
	} else if claims, ok := token.Claims.(*jwtAuth); ok {
		return claims
	} else {
		ja.logger.ErrorContext(r.Context(), "validation token is of unexpected type", "err", err)
		return nil
	}
}
