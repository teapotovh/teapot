package store

import (
	"time"

	flag "github.com/spf13/pflag"

	"github.com/teapotovh/teapot/lib/s3cache"
)

func StoreFlagSet() (*flag.FlagSet, func() StoreConfig) {
	fs := flag.NewFlagSet("mail/store", flag.ExitOnError)

	timeout := fs.Duration("mail-store-timeout", time.Minute, "timeout for store connection setup")
	typ := fs.String("mail-store-type", "mem", "the type of store to use. Options: mem, online")
	psqlURL := fs.String("mail-store-psql-url", "", "the URL connection string to connect to psql")
	s3URL := fs.String(
		"mail-store-s3-url",
		"http://key:secret@localhost:9000",
		"the URL connection string to connect to S3",
	)
	s3Region := fs.String("mail-store-s3-region", "garage", "the S3 region to connect to")
	s3Bucket := fs.String("mail-store-s3-bucket", "mail", "the S3 bucket used to store messages")

	s3CacheFS, getS3CacheConfig := s3cache.S3CacheFlagSet()
	fs.AddFlagSet(s3CacheFS)

	return fs, func() StoreConfig {
		return StoreConfig{
			Timeout: *timeout,
			Type:    *typ,
			URL:     *psqlURL,
			S3: S3Config{
				URL:    *s3URL,
				Region: *s3Region,
				Bucket: *s3Bucket,
				Cache:  getS3CacheConfig(),
			},
		}
	}
}
