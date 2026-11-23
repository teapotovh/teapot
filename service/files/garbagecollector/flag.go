package garbagecollector

import (
	flag "github.com/spf13/pflag"
)

// GarbageCollectorFlagSet returns the flags for a garbage collector config.
func GarbageCollectorFlagSet() (*flag.FlagSet, func() Config) {
	fs := flag.NewFlagSet("files/garbagecollector", flag.ExitOnError)

	url := fs.String("files-garbagecollector-psql-url", "", "PostgreSQL connection string")
	s3URL := fs.String("files-garbagecollector-s3", "", "S3 endpoint with optional credentials")
	s3Region := fs.String("files-garbagecollector-s3-region", "", "S3 region")
	s3Bucket := fs.String("files-garbagecollector-s3-bucket", "", "S3 bucket for file blobs")
	gracePeriod := fs.Duration(
		"files-garbagecollector-grace-period",
		DefaultGracePeriod,
		"minimum object age before deletion",
	)

	return fs, func() Config {
		return Config{
			URL: *url,
			S3: S3Config{
				URL:    *s3URL,
				Region: *s3Region,
				Bucket: *s3Bucket,
			},
			GracePeriod: *gracePeriod,
		}
	}
}
