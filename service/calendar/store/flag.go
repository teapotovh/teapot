package store

import (
	"time"

	flag "github.com/spf13/pflag"

	"github.com/teapotovh/teapot/lib/s3cache"
)

func StoreFlagSet() (*flag.FlagSet, func() StoreConfig) {
	fs := flag.NewFlagSet("calendar/store", flag.ExitOnError)

	timeout := fs.Duration("calendar-store-timeout", time.Minute, "timeout for store connection setup")
	typ := fs.String(
		"calendar-store-type",
		"mem",
		"the type of store to back the calendar server. Options: mem, online",
	)
	psqlURL := fs.String(
		"calendar-store-psql-url",
		"",
		"the URL connection string to connect to the store. mandatory for the online backend",
	)
	s3URL := fs.String(
		"calendar-store-s3-url",
		"http://key:secret@localhost:3900",
		"the URL connection string to connect to the S3 endpoint",
	)
	s3CacheFS, getS3CacheConfig := s3cache.S3CacheFlagSet()
	fs.AddFlagSet(s3CacheFS)

	return fs, func() StoreConfig {
		return StoreConfig{
			Timeout: *timeout,
			Type:    *typ,

			URL: *psqlURL,
			S3: StoreS3Config{
				URL:   *s3URL,
				Cache: getS3CacheConfig(),
			},
		}
	}
}
