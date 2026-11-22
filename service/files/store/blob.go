package store

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"github.com/teapotovh/teapot/lib/httptrace"
	"github.com/teapotovh/teapot/lib/observability"
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

// S3Blob stores and reads file bytes in S3.
type S3Blob struct {
	bucket string
	client *minio.Client
}

// NewS3Blob creates an S3 blob store.
func NewS3Blob(config S3Config) (*S3Blob, error) {
	u, err := url.Parse(config.URL)
	if err != nil {
		return nil, fmt.Errorf("error while parsing the S3 URL: %w", err)
	}

	key := u.User.Username()
	secret, _ := u.User.Password()

	httpTrace := httptrace.NewHTTPTrace()

	client, err := minio.New(u.Host, &minio.Options{
		Creds:     credentials.NewStaticV4(key, secret, ""),
		Secure:    u.Scheme == "https",
		Region:    config.Region,
		Transport: httpTrace.Transport(http.DefaultTransport),
	})
	if err != nil {
		return nil, fmt.Errorf("error while creating the S3 client: %w", err)
	}

	return &S3Blob{bucket: config.Bucket, client: client}, nil
}

// Put implements Blob.
func (s *S3Blob) Put(ctx context.Context, key string, r io.Reader) (Hash, error) {
	info, err := s.client.PutObject(ctx, s.bucket, key, r, -1, minio.PutObjectOptions{})
	if err != nil {
		return "", fmt.Errorf("error while storing blob: %w", err)
	}

	return Hash(info.ETag), nil
}

// Get implements Blob.
func (s *S3Blob) Get(ctx context.Context, key string) (io.ReadSeekCloser, error) {
	object, err := s.client.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("error while opening blob: %w", err)
	}

	return object, nil
}

// ReadinessChecks implements observability.ReadinessChecks.
func (s *S3Blob) ReadinessChecks() map[string]observability.Check {
	return map[string]observability.Check{
		"files/store/s3": observability.CheckFunc(s.bucketExists),
	}
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
