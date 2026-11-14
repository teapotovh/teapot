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
