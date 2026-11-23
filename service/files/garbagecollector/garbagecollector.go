package garbagecollector

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"log/slog"
	"time"

	"github.com/dustin/go-humanize"

	"github.com/teapotovh/teapot/lib/run"
	"github.com/teapotovh/teapot/lib/s3cache"
	"github.com/teapotovh/teapot/service/files/store"
)

const (
	// DefaultGracePeriod is the default minimum age of an object before deletion.
	DefaultGracePeriod = 10 * time.Minute

	deleteBatchSize = 1000
)

// Object is an S3 object.
type Object struct {
	// Key is the object key.
	Key string
	// ModTime is the last modification time.
	ModTime time.Time
	// Size is the object size in bytes.
	Size int64
}

// Refs provides the live blob refs.
type Refs interface {
	// LiveRefs returns all blob refs referenced by live file inodes.
	LiveRefs(ctx context.Context) ([]string, error)
	// Close closes the refs source.
	Close()
}

// Objects lists and deletes S3 objects.
type Objects interface {
	// List returns an iterator over all objects in the bucket.
	List(ctx context.Context) iter.Seq2[Object, error]
	// Delete removes the object with key.
	Delete(ctx context.Context, key string) error
}

// S3Config is the configuration for the S3 object store.
type S3Config struct {
	// URL is the S3 endpoint with optional credentials.
	URL string
	// Region is the S3 region.
	Region string
	// Bucket is the bucket that holds the blobs.
	Bucket string
}

// Config is the configuration for the garbage collector.
type Config struct {
	// URL is the PostgreSQL connection string.
	URL string
	// S3 is the S3 blob store configuration.
	S3 S3Config
	// GracePeriod is the minimum object age before deletion.
	GracePeriod time.Duration
}

// GarbageCollector removes unreferenced blobs from S3.
type GarbageCollector struct {
	logger *slog.Logger

	refs    Refs
	objects Objects

	gracePeriod time.Duration

	metrics metrics
}

type summary struct {
	deleted int
	bytes   int64
}

// NewGarbageCollector creates a garbage collector.
func NewGarbageCollector(
	ctx context.Context,
	config Config,
	logger *slog.Logger,
) (*GarbageCollector, error) {
	refs, err := store.NewPSQLDB(ctx, config.URL)
	if err != nil {
		return nil, err
	}

	client, err := s3cache.NewClient(config.S3.URL, config.S3.Region, nil)
	if err != nil {
		refs.Close()

		return nil, err
	}

	objects := &s3Objects{bucket: config.S3.Bucket, client: client}

	return newGarbageCollector(config, refs, objects, logger), nil
}

func newGarbageCollector(config Config, refs Refs, objects Objects, logger *slog.Logger) *GarbageCollector {
	collector := &GarbageCollector{
		logger: logger,

		refs:    refs,
		objects: objects,

		gracePeriod: config.GracePeriod,
	}

	collector.metrics.initMetrics()

	return collector
}

// Run implements run.Runnable.
func (c *GarbageCollector) Run(step run.Step) error {
	start := time.Now()
	defer c.metrics.observeRunDuration(start)
	defer c.refs.Close()

	step.Notify()

	result, err := c.collect(step.Context())
	c.logger.InfoContext(
		step.Context(),
		"garbage collection finished",
		"deleted_objects", result.deleted,
		"freed_bytes", result.bytes,
		"freed", humanize.IBytes(uint64(result.bytes)), //nolint:gosec // Object sizes are non-negative.
	)

	if err != nil {
		return err
	}

	return nil
}

func (c *GarbageCollector) collect(ctx context.Context) (summary, error) {
	var result summary

	refs, err := c.refs.LiveRefs(ctx)
	if err != nil {
		c.metrics.incErrors()

		return result, fmt.Errorf("error while reading live refs: %w", err)
	}

	live := make(map[string]struct{}, len(refs))
	for _, ref := range refs {
		live[ref] = struct{}{}
	}

	cutoff := time.Now().Add(-c.gracePeriod)
	batch := make([]Object, 0, deleteBatchSize)

	var errs []error

	flush := func() {
		for _, object := range batch {
			if err := c.objects.Delete(ctx, object.Key); err != nil {
				c.metrics.incErrors()
				c.logger.ErrorContext(ctx, "could not delete object", "key", object.Key, "err", err)
				errs = append(errs, err)

				continue
			}

			c.metrics.incDeletedObjects()
			c.logger.InfoContext(ctx, "deleted object", "key", object.Key)

			result.deleted++
			result.bytes += object.Size
		}

		batch = batch[:0]
	}

	for object, err := range c.objects.List(ctx) {
		if err != nil {
			c.metrics.incErrors()
			c.logger.ErrorContext(ctx, "could not list object", "err", err)
			errs = append(errs, err)

			continue
		}

		if !object.ModTime.Before(cutoff) {
			continue
		}

		if _, ok := live[object.Key]; ok {
			continue
		}

		batch = append(batch, object)
		if len(batch) >= deleteBatchSize {
			flush()
		}
	}

	flush()

	if len(errs) > 0 {
		return result, fmt.Errorf("error while collecting garbage: %w", errors.Join(errs...))
	}

	return result, nil
}

// Ensure the garbage collector implements run.Runnable.
var _ run.Runnable = (*GarbageCollector)(nil)
