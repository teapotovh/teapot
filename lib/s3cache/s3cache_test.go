package s3cache

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewS3CacheCreatesPath(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "cache", "s3")

	cache, err := NewS3Cache(
		S3CacheConfig{Bucket: "bucket", Path: path},
		nil,
		slog.New(slog.DiscardHandler),
	)
	require.NoError(t, err)
	require.NotNil(t, cache)

	info, err := os.Stat(path)
	require.NoError(t, err)
	require.True(t, info.IsDir())
}
