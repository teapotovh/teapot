package s3cache

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"uuid"

	"github.com/minio/minio-go/v7"
)

var (
	ErrS3CachePathRequired   = errors.New("path is required to initialize an s3cache")
	ErrS3CacheBucketRequired = errors.New("bucket is required to initialize an s3cache")

	DirFileMode = os.FileMode(0o0750)
)

// Blob stores and reads S3 objects.
type Blob interface {
	// Put stores the bytes read from r under key and returns their hash.
	Put(ctx context.Context, key string, r io.Reader) (Hash, error)
	// Get opens the object for key and returns its hash.
	Get(ctx context.Context, key string, expected Hash) (io.ReadSeekCloser, Hash, error)
	// Remove deletes key.
	Remove(ctx context.Context, key string) error
}

// Ensure S3Cache implements Blob.
var _ Blob = (*S3Cache)(nil)

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
}

// NewS3Cache creates an S3Cache.
func NewS3Cache(config S3CacheConfig, client *minio.Client, logger *slog.Logger) (*S3Cache, error) {
	if len(config.Path) <= 0 {
		return nil, ErrS3CachePathRequired
	}

	if len(config.Bucket) <= 0 {
		return nil, ErrS3CacheBucketRequired
	}

	if err := os.MkdirAll(config.Path, DirFileMode); err != nil {
		return nil, fmt.Errorf("error while creating the cache directory at %q: %w", config.Path, err)
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

		capacity: capacity,
	}

	return c, nil
}

// Get returns the content for key along with its current Hash (ETag).
// If a locally cached copy exists (whose hash matches the expected) it is
// returned without fetching from S3.
func (c *S3Cache) Get(ctx context.Context, key string, expected Hash) (io.ReadSeekCloser, Hash, error) {
	if !expected.IsZero() {
		file, ok, err := c.openCached(key, expected)
		if err != nil {
			return nil, ZeroHash, fmt.Errorf("error while reading (possibly) cached S3 key: %w", err)
		}

		if ok {
			c.logger.DebugContext(ctx, "cache hit", "key", key, "hash", expected)

			return file, expected, nil
		}
	}

	return c.fetch(ctx, key)
}

func (c *S3Cache) Put(ctx context.Context, key string, r io.Reader) (Hash, error) {
	file, err := os.CreateTemp(c.path, "upload-*")
	if err != nil {
		c.logger.WarnContext(ctx, "could not create temporary cache file", "key", key, "err", err)

		info, putErr := c.client.PutObject(ctx, c.bucket, key, r, -1, minio.PutObjectOptions{})
		if putErr != nil {
			return ZeroHash, fmt.Errorf("error while putting object %q: %w", key, putErr)
		}

		return Hash(info.ETag), nil
	}

	path := file.Name()

	info, err := c.client.PutObject(
		ctx,
		c.bucket,
		key,
		io.TeeReader(r, file),
		-1,
		minio.PutObjectOptions{},
	)
	if err != nil {
		return ZeroHash, fmt.Errorf(
			"error while putting object %q: %w",
			key,
			errors.Join(err, file.Close(), os.Remove(path)),
		)
	}

	hash := Hash(info.ETag)

	stat, err := file.Stat()
	if err != nil {
		c.logger.WarnContext(
			ctx,
			"failed to cache S3 object",
			"key", key,
			"err", errors.Join(err, file.Close(), os.Remove(path)),
		)

		return hash, nil
	}

	if err := file.Close(); err != nil {
		c.logger.WarnContext(
			ctx,
			"failed to cache S3 object",
			"key", key,
			"err", errors.Join(err, os.Remove(path)),
		)

		return hash, nil
	}

	if err := c.adopt(ctx, key, path, stat.Size(), hash); err != nil {
		c.logger.WarnContext(
			ctx,
			"failed to persist S3 cache entry",
			"key", key,
			"err", errors.Join(err, os.Remove(path)),
		)
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

func (c *S3Cache) openCached(key string, expected Hash) (*os.File, bool, error) {
	e := c.meta.get(key)
	if e == nil || e.Hash != expected {
		return nil, false, nil
	}

	file, err := os.Open(c.dataPath(e.ID))
	if err != nil {
		return nil, false, fmt.Errorf("error while reading from filesystem cache: %w", err)
	}

	c.meta.touch(key)

	return file, true, nil
}

func (c *S3Cache) fetch(ctx context.Context, key string) (io.ReadSeekCloser, Hash, error) {
	c.logger.DebugContext(ctx, "fetching from remote S3 store", "key", key)

	object, err := c.client.GetObject(ctx, c.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, ZeroHash, fmt.Errorf("error while getting object %q: %w", key, err)
	}

	info, err := object.Stat()
	if err != nil {
		return nil, ZeroHash, fmt.Errorf(
			"error while reading object metadata for %q: %w",
			key,
			errors.Join(err, object.Close()),
		)
	}

	hash := Hash(info.ETag)

	if info.Size >= c.capacity {
		return object, hash, nil
	}

	file, err := os.CreateTemp(c.path, "download-*")
	if err != nil {
		c.logger.WarnContext(ctx, "could not create temporary cache file", "key", key, "err", err)

		return object, hash, nil
	}

	return &download{
		ctx:       ctx,
		cache:     c,
		key:       key,
		hash:      hash,
		size:      info.Size,
		object:    object,
		file:      file,
		path:      file.Name(),
		cacheable: true,
	}, hash, nil
}

// Put stores the bytes read from r under key and returns their hash.
// The data is streamed to a temporary file before it is uploaded.
func (c *S3Cache) cap() int64 {
	return c.capacity - c.meta.metadataSize() - c.meta.cacheSize()
}

func (c *S3Cache) adopt(ctx context.Context, key string, path string, size int64, hash Hash) error {
	uid, oldSize := c.meta.resolveUID(key)

	// If this item is bigger than the whole cache, don't bother storing it.
	if size >= c.capacity {
		c.logger.DebugContext(
			ctx,
			"not caching s3 object as it exceeds the total cache size",
			"key",
			key,
			"size",
			size,
			"capacity",
			c.capacity,
		)

		return os.Remove(path)
	}

	if c.cap() < size-oldSize {
		if err := c.reclaimSpace(ctx, size-oldSize); err != nil {
			return fmt.Errorf("error while reclaiming cache space: %w", err)
		}
	}

	dataPath := c.dataPath(uid)
	if err := os.MkdirAll(filepath.Dir(dataPath), DirFileMode); err != nil {
		return fmt.Errorf("error while creating base directory for S3 cache entry: %w", err)
	}

	if err := os.Rename(path, dataPath); err != nil {
		return fmt.Errorf("error while moving S3 cache entry to disk: %w", err)
	}

	c.meta.put(key, uid, hash, size)

	if err := c.meta.persistLocked(); err != nil {
		return fmt.Errorf(
			"error while writing to s3 cache after adding new entry: %w",
			errors.Join(err, os.Remove(dataPath)),
		)
	}

	return nil
}

type download struct {
	ctx    context.Context
	cache  *S3Cache
	key    string
	hash   Hash
	size   int64
	object io.ReadSeekCloser
	file   *os.File
	path   string

	cacheable  bool
	done       bool
	closed     bool
	background bool
}

func (d *download) Read(p []byte) (int, error) {
	if d.done || d.background {
		return 0, io.EOF
	}

	n, err := d.object.Read(p)
	if n > 0 && d.cacheable {
		if _, writeErr := d.file.Write(p[:n]); writeErr != nil {
			d.disable()
		}
	}

	if err != nil {
		if errors.Is(err, io.EOF) {
			d.finish()
		} else {
			d.disable()
		}
	}

	return n, err
}

// Seek disables caching. A cached file is built from a sequential read only.
func (d *download) Seek(offset int64, whence int) (int64, error) {
	d.disable()
	d.done = false

	return d.object.Seek(offset, whence)
}

// Close keeps reading the object in the background when the caller stops early,
// so the whole object is cached.
func (d *download) Close() error {
	if d.done {
		return d.closeObject()
	}

	if d.background {
		return nil
	}

	if d.cacheable {
		d.background = true
		go d.complete()

		return nil
	}

	d.disable()

	return d.closeObject()
}

func (d *download) complete() {
	defer func() {
		if err := d.closeObject(); err != nil {
			d.cache.logger.WarnContext(d.ctx, "could not close S3 object", "key", d.key, "err", err)
		}
	}()

	buffer := make([]byte, 32<<10)

	for {
		n, err := d.object.Read(buffer)
		if n > 0 && d.cacheable {
			if _, writeErr := d.file.Write(buffer[:n]); writeErr != nil {
				d.disable()

				return
			}
		}

		if err != nil {
			if errors.Is(err, io.EOF) {
				d.finish()
			} else {
				d.disable()
			}

			return
		}
	}
}

func (d *download) closeObject() error {
	if d.closed {
		return nil
	}

	d.closed = true

	return d.object.Close()
}

func (d *download) disable() {
	if !d.cacheable {
		return
	}

	d.cacheable = false

	if err := d.file.Close(); err != nil {
		d.cache.logger.WarnContext(d.ctx, "could not close download file", "key", d.key, "err", err)
	}

	if err := os.Remove(d.path); err != nil && !errors.Is(err, os.ErrNotExist) {
		d.cache.logger.WarnContext(d.ctx, "could not remove download file", "key", d.key, "err", err)
	}
}

func (d *download) finish() {
	if d.done {
		return
	}

	d.done = true

	cacheable := d.cacheable
	d.cacheable = false

	if cacheable {
		info, statErr := d.file.Stat()
		if statErr == nil && info.Size() == d.size {
			if err := d.file.Close(); err == nil {
				if err := d.cache.adopt(d.ctx, d.key, d.path, d.size, d.hash); err != nil {
					removeErr := os.Remove(d.path)
					d.cache.logger.WarnContext(
						d.ctx,
						"failed to persist S3 cache entry",
						"key", d.key,
						"err", errors.Join(err, removeErr),
					)
				}

				return
			} else {
				d.cache.logger.WarnContext(d.ctx, "could not close download file", "key", d.key, "err", err)
			}
		} else if statErr != nil {
			d.cache.logger.WarnContext(d.ctx, "could not stat download file", "key", d.key, "err", statErr)
		}
	}

	if err := d.file.Close(); err != nil {
		d.cache.logger.WarnContext(d.ctx, "could not close download file", "key", d.key, "err", err)
	}

	if err := os.Remove(d.path); err != nil && !errors.Is(err, os.ErrNotExist) {
		d.cache.logger.WarnContext(d.ctx, "could not remove download file", "key", d.key, "err", err)
	}
}

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
