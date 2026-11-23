package store

import (
	flag "github.com/spf13/pflag"

	"github.com/teapotovh/teapot/lib/s3cache"
)

// StoreFlagSet registers the store flags.
func StoreFlagSet() (*flag.FlagSet, func() Config) {
	fs := flag.NewFlagSet("files/store", flag.ExitOnError)

	psqlURL := fs.String("files-store-psql-url", "", "PostgreSQL connection string for the file store")

	cacheFS, getCacheConfig := CacheFlagSet()
	fs.AddFlagSet(cacheFS)

	s3FS, getS3Config := S3FlagSet()
	fs.AddFlagSet(s3FS)

	return fs, func() Config {
		return Config{
			URL:   *psqlURL,
			Cache: getCacheConfig(),
			S3:    getS3Config(),
		}
	}
}

// S3FlagSet returns the flags for an S3 blob store config.
func S3FlagSet() (*flag.FlagSet, func() S3Config) {
	fs := flag.NewFlagSet("files/store/s3", flag.ExitOnError)

	url := fs.String("files-store-s3-url", "", "S3 endpoint with optional credentials")
	region := fs.String("files-store-s3-region", "", "S3 region")

	s3CacheFS, getS3CacheConfig := s3cache.S3CacheFlagSet()
	fs.AddFlagSet(s3CacheFS)

	return fs, func() S3Config {
		return S3Config{
			URL:    *url,
			Region: *region,
			Cache:  getS3CacheConfig(),
		}
	}
}
