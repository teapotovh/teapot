package s3cache

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDownloadCachesFullRead(t *testing.T) {
	t.Parallel()

	cache := newTestCache(t)
	data := []byte("hello world")
	download := newTestDownload(t, cache, "key", data, "etag")

	got, err := io.ReadAll(download)
	require.NoError(t, err)
	require.Equal(t, data, got)
	require.NoError(t, download.Close())

	entry := cache.meta.get("key")
	require.NotNil(t, entry)
	require.Equal(t, Hash("etag"), entry.Hash)
	require.EqualValues(t, len(data), entry.Size)

	reader, hash, err := cache.Get(context.Background(), "key", "etag")
	require.NoError(t, err)
	require.Equal(t, Hash("etag"), hash)

	defer func() {
		require.NoError(t, reader.Close())
	}()

	cached, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.Equal(t, data, cached)
}

func TestDownloadCachesRestAfterEarlyClose(t *testing.T) {
	t.Parallel()

	cache := newTestCache(t)
	data := []byte("hello world")
	download := newTestDownload(t, cache, "key", data, "etag")

	prefix := make([]byte, 5)
	_, err := io.ReadFull(download, prefix)
	require.NoError(t, err)
	require.Equal(t, data[:5], prefix)
	require.NoError(t, download.Close())

	require.Eventually(t, func() bool {
		return cache.meta.get("key") != nil
	}, time.Second, time.Millisecond)

	reader, _, err := cache.Get(context.Background(), "key", "etag")
	require.NoError(t, err)

	defer func() {
		require.NoError(t, reader.Close())
	}()

	cached, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.Equal(t, data, cached)
}

func TestDownloadSeekDoesNotCache(t *testing.T) {
	t.Parallel()

	cache := newTestCache(t)
	data := []byte("hello world")
	download := newTestDownload(t, cache, "key", data, "etag")

	position, err := download.Seek(6, io.SeekStart)
	require.NoError(t, err)
	require.EqualValues(t, 6, position)

	got, err := io.ReadAll(download)
	require.NoError(t, err)
	require.Equal(t, data[6:], got)
	require.NoError(t, download.Close())

	require.Nil(t, cache.meta.get("key"))
}

func newTestCache(t *testing.T) *S3Cache {
	t.Helper()

	path := t.TempDir()

	meta, err := newMetadataStore(path)
	require.NoError(t, err)

	return &S3Cache{
		logger:   slog.New(slog.DiscardHandler),
		path:     path,
		capacity: 1 << 20,
		meta:     meta,
	}
}

func newTestDownload(t *testing.T, cache *S3Cache, key string, data []byte, hash Hash) *download {
	t.Helper()

	file, err := os.CreateTemp(cache.path, "test-*")
	require.NoError(t, err)

	return &download{
		ctx:       context.Background(),
		cache:     cache,
		key:       key,
		hash:      hash,
		size:      int64(len(data)),
		object:    &testObject{Reader: bytes.NewReader(data)},
		file:      file,
		path:      file.Name(),
		cacheable: true,
	}
}

type testObject struct {
	*bytes.Reader
}

func (o *testObject) Close() error {
	return nil
}
