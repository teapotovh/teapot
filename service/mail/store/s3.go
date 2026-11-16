package store

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"github.com/teapotovh/teapot/lib/httptrace"
	"github.com/teapotovh/teapot/lib/s3cache"
)

type blobStore interface {
	Put(ctx context.Context, data []byte) ([]byte, error)
	Get(ctx context.Context, digest []byte) ([]byte, error)
	Remove(ctx context.Context, digest []byte) error
}

type s3Blobs struct {
	client *minio.Client
	bucket string
	cache  *s3cache.S3Cache
}

func blobKey(digest []byte) string {
	raw := hex.EncodeToString(digest)

	return "mail/" + raw[:2] + "/" + raw[2:4] + "/" + raw
}

func newS3Blobs(ctx context.Context, config S3Config, logger *slog.Logger) (*s3Blobs, error) {
	if config.Cache.Bucket == "" {
		config.Cache.Bucket = config.Bucket
	}

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

	exists, err := client.BucketExists(ctx, config.Bucket)
	if err != nil {
		return nil, fmt.Errorf("error while checking the S3 bucket: %w", err)
	}

	if !exists {
		if err := client.MakeBucket(ctx, config.Bucket, minio.MakeBucketOptions{Region: config.Region}); err != nil {
			return nil, fmt.Errorf("error while creating the S3 bucket: %w", err)
		}
	}

	if err := os.MkdirAll(config.Cache.Path, 0o750); err != nil {
		return nil, fmt.Errorf("error while creating the S3 cache directory: %w", err)
	}

	cache, err := s3cache.NewS3Cache(config.Cache, client, logger)
	if err != nil {
		return nil, fmt.Errorf("error while building the S3 cache: %w", err)
	}

	return &s3Blobs{client: client, bucket: config.Bucket, cache: cache}, nil
}

func (s *s3Blobs) Put(ctx context.Context, data []byte) ([]byte, error) {
	digest := digestBytes(data)
	key := blobKey(digest)

	if _, err := s.cache.Put(ctx, key, data); err != nil {
		return nil, fmt.Errorf("error while storing the message body in S3: %w", err)
	}

	return digest, nil
}

func (s *s3Blobs) Get(ctx context.Context, digest []byte) ([]byte, error) {
	data, _, err := s.cache.Get(ctx, blobKey(digest), s3cache.ZeroHash)
	if err != nil {
		return nil, fmt.Errorf("error while reading the message body from S3: %w", err)
	}

	if !bytes.Equal(digestBytes(data), digest) {
		return nil, ErrChecksumMismatch
	}

	return data, nil
}

func (s *s3Blobs) Remove(ctx context.Context, digest []byte) error {
	if err := s.cache.Remove(ctx, blobKey(digest)); err != nil {
		return fmt.Errorf("error while removing the message body from S3: %w", err)
	}

	return nil
}
