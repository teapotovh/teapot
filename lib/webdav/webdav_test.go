package webdav

import (
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	daverr "github.com/teapotovh/teapot/lib/webdav/error"
)

var errTestCause = errors.New("boom")

func TestConditionalMatch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		match     ConditionalMatch
		isSet     bool
		isWild    bool
		etag      string
		etagErr   bool
		matchETag string
		matched   bool
		matchErr  bool
	}{
		{
			name:      "empty",
			match:     "",
			isSet:     false,
			isWild:    false,
			etagErr:   true,
			matchETag: `"abc"`,
			matched:   false,
		},
		{
			name:      "wildcard",
			match:     "*",
			isSet:     true,
			isWild:    true,
			etagErr:   true,
			matchETag: `"abc"`,
			matched:   true,
		},
		{
			name:      "wildcard matches unquoted value",
			match:     "*",
			isSet:     true,
			isWild:    true,
			etagErr:   true,
			matchETag: "abc",
			matched:   true,
		},
		{
			name:      "strong etag matches",
			match:     `"abc"`,
			isSet:     true,
			isWild:    false,
			etag:      "abc",
			matchETag: "abc",
			matched:   true,
		},
		{
			name:      "strong etag mismatch",
			match:     `"abc"`,
			isSet:     true,
			isWild:    false,
			etag:      "abc",
			matchETag: "def",
			matched:   false,
		},
		{
			name:      "malformed etag",
			match:     "abc",
			isSet:     true,
			isWild:    false,
			etagErr:   true,
			matchETag: "abc",
			matched:   false,
			matchErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.isSet, tt.match.IsSet())
			assert.Equal(t, tt.isWild, tt.match.IsWildcard())

			etag, err := tt.match.ETag()
			if tt.etagErr {
				require.Error(t, err)
				assert.Empty(t, etag)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.etag, etag)
			}

			matched, err := tt.match.MatchETag(tt.matchETag)
			if tt.matchErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			assert.Equal(t, tt.matched, matched)
		})
	}
}

func TestNewHTTPError(t *testing.T) {
	t.Parallel()

	cause := errTestCause

	err := NewHTTPError(http.StatusNotFound, cause)
	require.Error(t, err)
	assert.ErrorIs(t, err, cause)

	httpErr := daverr.HTTPErrorFromError(err)
	require.NotNil(t, httpErr)
	assert.Equal(t, http.StatusNotFound, httpErr.Code)
	assert.Equal(t, cause, httpErr.Err)

	assert.Equal(t, "404 Not Found: boom", err.Error())

	err = NewHTTPError(http.StatusConflict, nil)
	require.Error(t, err)

	httpErr = daverr.HTTPErrorFromError(err)
	require.NotNil(t, httpErr)
	assert.Equal(t, http.StatusConflict, httpErr.Code)
	assert.NoError(t, httpErr.Err)
	assert.Equal(t, "409 Conflict", err.Error())
}
