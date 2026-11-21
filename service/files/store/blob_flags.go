package store

import (
	flag "github.com/spf13/pflag"
)

// S3FlagSet returns the flags for an S3 blob store config.
func S3FlagSet() (*flag.FlagSet, func() S3Config) {
	fs := flag.NewFlagSet("files/store/s3", flag.ExitOnError)

	url := fs.String("files-store-s3", "", "S3 endpoint with optional credentials")
	region := fs.String("files-store-s3-region", "", "S3 region")
	bucket := fs.String("files-store-s3-bucket", "", "S3 bucket for file blobs")

	return fs, func() S3Config {
		return S3Config{
			URL:    *url,
			Region: *region,
			Bucket: *bucket,
		}
	}
}
