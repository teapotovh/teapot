package store

import (
	flag "github.com/spf13/pflag"
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
