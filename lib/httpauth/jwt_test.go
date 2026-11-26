package httpauth

import (
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestAuthCookieCSRFProtection ensures the authentication cookies keep the
// attributes that block cross-site form posts.
func TestAuthCookieCSRFProtection(t *testing.T) {
	t.Parallel()

	ja := NewJWTAuth(nil, JWTAuthConfig{
		Secret:   "secret",
		Issuser:  "issuer",
		Duration: time.Hour,
		Prefix:   "/",
		Secure:   true,
	}, slog.New(slog.DiscardHandler))

	authCookie, err := ja.authCookie("user", false)
	require.NoError(t, err)

	logoutCookie := ja.DeAuthenticate()

	for _, cookie := range []*http.Cookie{authCookie, &logoutCookie} {
		require.Equal(t, cookieName, cookie.Name)
		require.Equal(t, "/", cookie.Path)
		require.True(t, cookie.HttpOnly)
		require.True(t, cookie.Secure)
		require.Equal(t, http.SameSiteStrictMode, cookie.SameSite)
	}
}
