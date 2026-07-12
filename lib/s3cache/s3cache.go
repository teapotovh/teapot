package s3cache

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
	"golang.org/x/sync/singleflight"
)

var (
	S3CachePathRequired   = errors.New("path is required to initialize an s3cache")
	S3CacheBucketRequired = errors.New("bucket is required to initialize an s3cache")

	DirFileMode = os.FileMode(0o0750)
)

// S3CacheConfig is the config for a new S3Cache.
type S3CacheConfig struct {
	// Bucket is the S3 bucket objects are read from.
	Bucket string

	// Path is the local directory used for cache storage. It should live on a
	// volume dedicated to this cache.
	Path string
}

// S3Cache is a read-through local disk cache in front of S3.
type S3Cache struct {
	logger *slog.Logger

	path   string
	bucket string

	client *minio.Client
	meta   *metadataStore

	capacity int64

	evictMu sync.Mutex
	sfg     singleflight.Group
}

// NewS3Cache creates an S3Cache.
func NewS3Cache(config S3CacheConfig, client *minio.Client, logger *slog.Logger) (*S3Cache, error) {
	if len(config.Path) <= 0 {
		return nil, S3CachePathRequired
	}

	if len(config.Bucket) <= 0 {
		return nil, S3CacheBucketRequired
	}

	capacity, err := diskCapacity(config.Path)
	if err != nil {
		return nil, fmt.Errorf("error while fetching the cache size at %q: %w", config.Path, err)
	}

	meta, err := newMetadataStore(config.Path)
	if err != nil {
		return nil, fmt.Errorf("error while loading the metadata store: %w", err)
	}

	c := &S3Cache{
		logger: logger,

		path:   config.Path,
		bucket: config.Bucket,

		client: client,
		meta:   meta,

		capacity: int64(capacity),
	}

	return c, nil
}

// Get returns the content for key along with its current Hash (ETag).
// If a locally cached copy exists (whose hash matches the expected) it's
// returned without fetching from S3.
func (c *S3Cache) Get(ctx context.Context, key string, expected Hash) ([]byte, Hash, error) {
	if !expected.IsZero() {
		data, ok, err := c.readCached(key, expected)
		if err != nil {
			return nil, ZeroHash, fmt.Errorf("error while reading (possibly) cached S3 key: %w", err)
		}

		if ok {
			c.logger.DebugContext(ctx, "cache hit", "key", key, "hash", expected, "size", len(data))
			return data, expected, nil
		}
	}

	type result struct {
		data []byte
		hash Hash
	}

	v, err, _ := c.sfg.Do(key, func() (any, error) {
		data, hash, err := c.fetchAndStore(ctx, key)
		if err != nil {
			return nil, err
		}

		return result{data: data, hash: hash}, nil
	})
	if err != nil {
		return nil, ZeroHash, fmt.Errorf("error while fetching key from remote S3: %w", err)
	}

	r := v.(result)

	return r.data, r.hash, nil
}

// Put uploads data to S3 under key, then caches it locally with the resulting
// ETag, returning that Hash for the caller to use.
func (c *S3Cache) Put(ctx context.Context, key string, data []byte) (Hash, error) {
	info, err := c.client.PutObject(
		ctx,
		c.bucket,
		key,
		bytes.NewReader(data),
		int64(len(data)),
		minio.PutObjectOptions{},
	)
	if err != nil {
		return ZeroHash, fmt.Errorf("error while putting object %q: %w", key, err)
	}

	hash := Hash(info.ETag)

	if err := c.store(ctx, key, data, hash); err != nil {
		c.logger.WarnContext(ctx, "failed to persist S3 cache entry", "key", key, "err", err)
	}

	return hash, nil
}

// Remove removes key from S3 and, if present, from the local cache.
func (c *S3Cache) Remove(ctx context.Context, key string) error {
	if err := c.client.RemoveObject(ctx, c.bucket, key, minio.RemoveObjectOptions{}); err != nil {
		return fmt.Errorf("error while deleting object %q: %w", key, err)
	}

	if candidate := c.meta.get(key); candidate != nil {
		c.meta.remove(key)

		if err := c.meta.persistLocked(); err != nil {
			return fmt.Errorf("error while removing s3 cache entry: %w", err)
		}

		dataPath := c.dataPath(candidate.ID)
		if err := os.Remove(dataPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			c.logger.Warn("error while removing S3 cached file", "key", key, "err", err)
		}
	}

	return nil
}

func (c *S3Cache) cap() int64 {
	return c.capacity - c.meta.metadataSize() - c.meta.cacheSize()
}

func (c *S3Cache) readCached(key string, expected Hash) ([]byte, bool, error) {
	e := c.meta.get(key)
	if e == nil || e.Hash != expected {
		return nil, false, nil
	}

	data, err := os.ReadFile(c.dataPath(e.ID))
	if err != nil {
		return nil, false, fmt.Errorf("error while reading from filesystem cache: %w", err)
	}

	c.meta.touch(key)

	return data, true, nil
}

func (c *S3Cache) fetchAndStore(ctx context.Context, key string) ([]byte, Hash, error) {
	obj, err := c.client.GetObject(ctx, c.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, ZeroHash, fmt.Errorf("error while getting object %q: %w", key, err)
	}
	defer obj.Close()

	info, err := obj.Stat()
	if err != nil {
		return nil, ZeroHash, fmt.Errorf("error while statting on object %q: %w", key, err)
	}

	hash := Hash(info.ETag)

	data, err := io.ReadAll(obj)
	if err != nil {
		return nil, ZeroHash, fmt.Errorf("error while reading reading object %q: %w", key, err)
	}

	if err := c.store(ctx, key, data, hash); err != nil {
		c.logger.WarnContext(ctx, "failed to persist S3 cache entry", "key", key, "err", err)
	}

	return data, hash, nil
}

func (c *S3Cache) store(ctx context.Context, key string, data []byte, hash Hash) error {
	uid, oldSize := c.meta.resolveUID(key)
	newSize := int64(len(data))
	sizeDiff := newSize - oldSize

	// If this item is bigger than the whole cache, don't bother storing it
	if newSize >= c.capacity {
		c.logger.DebugContext(
			ctx,
			"not caching s3 object as it exceeds the total cache size",
			"key",
			key,
			"size",
			newSize,
			"capacity",
			c.capacity,
		)
		return nil
	}

	// If we don't have enough space to accommodate the new entry, remove enough
	// entries to satisfy this in LRU fashion.
	if c.cap() < sizeDiff {
		if err := c.reclaimSpace(ctx, sizeDiff); err != nil {
			return fmt.Errorf("error while reclaiming cache space: %w", err)
		}
	}

	dataPath := c.dataPath(uid)
	if err := os.MkdirAll(filepath.Dir(dataPath), DirFileMode); err != nil {
		return fmt.Errorf("error while creating base directory for S3 cache entry: %w", err)
	}

	if err := writeFileAtomic(dataPath, data); err != nil {
		return fmt.Errorf("error while writing S3 cache entry to disk: %w", err)
	}

	c.meta.put(key, uid, hash, newSize)

	if err := c.meta.persistLocked(); err != nil {
		return fmt.Errorf("error while writing to s3 cache after adding new entry: %w", err)
	}

	return nil
}

// dataPath returns where a data file for uid lives on disk. Sharded by
// the first two hex chars of the UID purely to keep any single directory
// from growing unbounded -- this is an internal storage-layout detail,
// not derived from the cache key.
func (c *S3Cache) dataPath(id uuid.UUID) string {
	uid := id.String()
	return filepath.Join(c.path, uid[:2], uid[2:])
}

// reclaimSpace trims the cache back by at least diff bytes, LRU first.
func (c *S3Cache) reclaimSpace(ctx context.Context, diff int64) error {
	c.evictMu.Lock()
	defer c.evictMu.Unlock()

	// First, we want to determine which entries to remove and update the metadata.
	// Secondly, we will remove those files. This prevents having cache entries
	// in the metadata that point nowhere.

	heap := c.meta.lruHeap()
	next := 0
	freed := int64(0)

	var drop []snapshot

	for next < len(heap) && freed < diff {
		candidate := heap[next]
		drop = append(drop, candidate)

		c.logger.DebugContext(ctx, "evicting entry", "key", candidate.Key, "id", candidate.ID, "size", candidate.Size)
		c.meta.remove(candidate.Key)

		freed -= candidate.Size
		next += 1
	}

	if err := c.meta.persistLocked(); err != nil {
		return fmt.Errorf("error while writing to s3 cache after removing %d entries: %w", len(drop), err)
	}

	for _, candidate := range drop {
		dataPath := c.dataPath(candidate.ID)
		if err := os.Remove(dataPath); err != nil {
			return fmt.Errorf("error while removing S3 cached key (file): %w", err)
		}
	}

	return nil
}
