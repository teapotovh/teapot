package store

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/minio/minio-go/v7"

	"github.com/teapotovh/teapot/lib/observability"
	"github.com/teapotovh/teapot/lib/s3cache"
)

// S3Config is the configuration for the S3 blob store.
type S3Config struct {
	// URL is the S3 endpoint with optional credentials.
	URL string
	// Region is the S3 region.
	Region string
	// Bucket is the bucket that holds the blobs.
	Bucket string
}

// ErrBucketNotFound reports that the configured S3 bucket does not exist.
var ErrBucketNotFound = errors.New("bucket not found")

// S3Blob stores and reads file bytes in S3 without a local cache.
type S3Blob struct {
	bucket string
	client *minio.Client
}

// NewS3Blob creates an S3 blob store.
func NewS3Blob(config S3Config) (*S3Blob, error) {
	client, err := s3cache.NewClient(config.URL, config.Region, nil)
	if err != nil {
		return nil, err
	}

	return &S3Blob{bucket: config.Bucket, client: client}, nil
}

// Put implements s3cache.Blob.
func (s *S3Blob) Put(ctx context.Context, key string, r io.Reader) (s3cache.Hash, error) {
	info, err := s.client.PutObject(ctx, s.bucket, key, r, -1, minio.PutObjectOptions{})
	if err != nil {
		return s3cache.ZeroHash, fmt.Errorf("error while storing object %q: %w", key, err)
	}

	return s3cache.Hash(info.ETag), nil
}

// Get implements s3cache.Blob.
func (s *S3Blob) Get(
	ctx context.Context,
	key string,
	_ s3cache.Hash,
) (io.ReadSeekCloser, s3cache.Hash, error) {
	object, err := s.client.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, s3cache.ZeroHash, fmt.Errorf("error while opening object %q: %w", key, err)
	}

	info, err := object.Stat()
	if err != nil {
		return nil, s3cache.ZeroHash, fmt.Errorf(
			"error while reading object metadata for %q: %w",
			key,
			errors.Join(err, object.Close()),
		)
	}

	return object, s3cache.Hash(info.ETag), nil
}

// ReadinessChecks implements observability.ReadinessChecks.
func (s *S3Blob) ReadinessChecks() map[string]observability.Check {
	return map[string]observability.Check{
		"files/store/s3": observability.CheckFunc(s.bucketExists),
	}
}

// Remove implements s3cache.Blob.
func (s *S3Blob) Remove(ctx context.Context, key string) error {
	if err := s.client.RemoveObject(ctx, s.bucket, key, minio.RemoveObjectOptions{}); err != nil {
		return fmt.Errorf("error while removing object %q: %w", key, err)
	}

	return nil
}

func (s *S3Blob) bucketExists(ctx context.Context) error {
	exists, err := s.client.BucketExists(ctx, s.bucket)
	if err != nil {
		return fmt.Errorf("error while checking the S3 bucket: %w", err)
	}

	if !exists {
		return fmt.Errorf("%w: %q", ErrBucketNotFound, s.bucket)
	}

	return nil
}

// Ensure S3Blob implements s3cache.Blob.
var _ s3cache.Blob = (*S3Blob)(nil)
