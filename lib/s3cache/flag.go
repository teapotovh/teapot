package s3cache

import (
	flag "github.com/spf13/pflag"
)

func S3CacheFlagSet() (*flag.FlagSet, func() S3CacheConfig) {
	fs := flag.NewFlagSet("s3cache", flag.ExitOnError)

	bucket := fs.String("s3cache-bucket", "", "the S3 bucket to read data from")
	path := fs.String("s3cache-path", "/s3cache", "the path where the S3 data is cached")

	return fs, func() S3CacheConfig {
		return S3CacheConfig{
			Path:   *path,
			Bucket: *bucket,
		}
	}
}
