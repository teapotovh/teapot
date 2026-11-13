package caldav

import (
	"embed"
	"testing"

	"github.com/stretchr/testify/require"
)

//go:embed testdata/*
var testdataFS embed.FS

func testdata(t *testing.T, name string) string {
	t.Helper()

	content, err := testdataFS.ReadFile("testdata/" + name)
	require.NoError(t, err)

	return string(content)
}
