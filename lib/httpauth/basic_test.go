package httpauth

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/teapotovh/teapot/lib/ldap"
)

var errUnexpected = errors.New("unexpected error")

type fakeAuthenticator struct {
	users map[string]string
	err   error
}

func (f *fakeAuthenticator) Authenticate(_ context.Context, username, password string) (*ldap.User, error) {
	if f.err != nil {
		return nil, f.err
	}

	if expected, ok := f.users[username]; !ok || expected != password {
		return nil, ldap.ErrInvalidCredentials
	}

	return &ldap.User{Username: username}, nil
}

func TestBasicAuthStatus(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.DiscardHandler)
	users := map[string]string{"user": "password"}

	tests := []struct {
		name     string
		auth     ldap.Authenticator
		username string
		password string
		authSet  bool
		want     int
	}{
		{
			name: "missing credentials",
			auth: &fakeAuthenticator{users: users},
			want: http.StatusUnauthorized,
		},
		{
			name:     "invalid credentials",
			auth:     &fakeAuthenticator{users: users},
			username: "user",
			password: "wrong",
			authSet:  true,
			want:     http.StatusUnauthorized,
		},
		{
			name:     "unknown user",
			auth:     &fakeAuthenticator{users: users},
			username: "unknown",
			password: "password",
			authSet:  true,
			want:     http.StatusUnauthorized,
		},
		{
			name:     "valid credentials",
			auth:     &fakeAuthenticator{users: users},
			username: "user",
			password: "password",
			authSet:  true,
			want:     http.StatusOK,
		},
		{
			name:     "unexpected error",
			auth:     &fakeAuthenticator{err: errUnexpected},
			username: "user",
			password: "password",
			authSet:  true,
			want:     http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})

			ba := NewBasicAuth(tt.auth, DefaultBasicAuthErrorHandler, logger)

			srv := httptest.NewServer(ba.Middleware(ba.Required(handler)))
			defer srv.Close()

			req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)
			require.NoError(t, err)

			if tt.authSet {
				req.SetBasicAuth(tt.username, tt.password)
			}

			resp, err := http.DefaultClient.Do(req)
			require.NoError(t, err)

			t.Cleanup(func() { _ = resp.Body.Close() })

			require.Equal(t, tt.want, resp.StatusCode)

			if tt.want == http.StatusUnauthorized {
				require.NotEmpty(t, resp.Header.Get("WWW-Authenticate"))
			}
		})
	}
}
