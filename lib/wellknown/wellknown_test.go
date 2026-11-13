package wellknown

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var errResolverFailed = errors.New("resolver failed")

func TestResolverHostServerRequest(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, Prefix+string(Caldav), nil)

	path, err := ResolverHost(req)
	require.NoError(t, err)

	// Server-side requests do not carry a scheme, so the resulting location is
	// a scheme-relative URL pointing at the origin root.
	assert.Equal(t, "//example.com", path)
}

func TestResolverHostAbsoluteURL(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "https://calendar.example.com"+Prefix+string(Caldav), nil)

	path, err := ResolverHost(req)
	require.NoError(t, err)

	assert.Equal(t, "https://calendar.example.com", path)
}

func TestWellKnownServeHTTP(t *testing.T) {
	t.Parallel()

	wellKnown := WellKnown{
		Caldav: ResolverHost,
	}

	tests := []struct {
		name     string
		path     string
		status   int
		location string
	}{
		{
			name:     "caldav redirects to the origin",
			path:     Prefix + string(Caldav),
			status:   http.StatusPermanentRedirect,
			location: "//example.com",
		},
		{
			name:   "unregistered service",
			path:   Prefix + "carddav",
			status: http.StatusNotFound,
		},
		{
			name:   "empty service",
			path:   Prefix,
			status: http.StatusNotFound,
		},
		{
			name:   "missing prefix",
			path:   "/foo",
			status: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			rec := httptest.NewRecorder()

			wellKnown.ServeHTTP(rec, req)

			resp := rec.Result()
			defer func() { _ = resp.Body.Close() }()

			require.Equal(t, tt.status, resp.StatusCode)
			assert.Equal(t, tt.location, resp.Header.Get("Location"))
		})
	}
}

func TestWellKnownResolverError(t *testing.T) {
	t.Parallel()

	wellKnown := WellKnown{
		Caldav: func(*http.Request) (string, error) {
			return "", errResolverFailed
		},
	}

	req := httptest.NewRequest(http.MethodGet, Prefix+string(Caldav), nil)
	rec := httptest.NewRecorder()

	wellKnown.ServeHTTP(rec, req)

	resp := rec.Result()
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
}
