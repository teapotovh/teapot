package httpauth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestNewAuthContext ensures the constructed context carries the auth value
// so that both GetAuthContext and GetAuth can retrieve it again.
func TestNewAuthContext(t *testing.T) {
	t.Parallel()

	auth := &Auth{Username: "user"}

	ctx := NewAuthContext(context.Background(), auth)
	assert.Same(t, auth, GetAuthContext(ctx))

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r = r.WithContext(NewAuthContext(r.Context(), auth))
	assert.Same(t, auth, GetAuth(r))
}

// TestGetAuthContextWithoutAuth ensures a context without an auth value
// yields nil instead of panicking.
func TestGetAuthContextWithoutAuth(t *testing.T) {
	t.Parallel()

	assert.Nil(t, GetAuthContext(context.Background()))
}
