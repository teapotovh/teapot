package garbagecollector

import (
	"context"
	"errors"
	"fmt"
	"iter"

	"github.com/minio/minio-go/v7"

	"github.com/teapotovh/teapot/lib/observability"
)

// ErrBucketNotFound reports that the configured S3 bucket does not exist.
var ErrBucketNotFound = errors.New("bucket not found")

type s3Objects struct {
	client *minio.Client
	bucket string
}

// List implements Objects.
func (s *s3Objects) List(ctx context.Context) iter.Seq2[Object, error] {
	return func(yield func(Object, error) bool) {
		for info := range s.client.ListObjects(ctx, s.bucket, minio.ListObjectsOptions{Recursive: true}) {
			if info.Err != nil {
				if !yield(Object{}, fmt.Errorf("error while listing objects: %w", info.Err)) {
					return
				}

				continue
			}

			if !yield(Object{Key: info.Key, ModTime: info.LastModified, Size: info.Size}, nil) {
				return
			}
		}
	}
}

// Delete implements Objects.
func (s *s3Objects) Delete(ctx context.Context, key string) error {
	if err := s.client.RemoveObject(ctx, s.bucket, key, minio.RemoveObjectOptions{}); err != nil {
		return fmt.Errorf("error while removing object %q: %w", key, err)
	}

	return nil
}

// ReadinessChecks implements observability.ReadinessChecks.
func (s *s3Objects) ReadinessChecks() map[string]observability.Check {
	return map[string]observability.Check{
		"files/garbagecollector/s3": observability.CheckFunc(s.bucketExists),
	}
}

func (s *s3Objects) bucketExists(ctx context.Context) error {
	exists, err := s.client.BucketExists(ctx, s.bucket)
	if err != nil {
		return fmt.Errorf("error while checking the S3 bucket: %w", err)
	}

	if !exists {
		return fmt.Errorf("%w: %q", ErrBucketNotFound, s.bucket)
	}

	return nil
}

// Ensure s3Objects implements Objects.
var _ Objects = (*s3Objects)(nil)
