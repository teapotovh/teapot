package staticsite

import (
	"time"

	flag "github.com/spf13/pflag"

	"github.com/teapotovh/teapot/lib/httplog"
	"github.com/teapotovh/teapot/lib/ldap"
	"github.com/teapotovh/teapot/lib/remotefiles"
	"github.com/teapotovh/teapot/lib/s3cache"
)

// StaticSiteFlagSet returns the flags for a static site config.
func StaticSiteFlagSet() (*flag.FlagSet, func() Config) {
	fs := flag.NewFlagSet("staticsite", flag.ExitOnError)

	httpLogFS, getHTTPLogConfig := httplog.HTTPLogFlagSet()
	fs.AddFlagSet(httpLogFS)

	ldapFS, getLDAPConfig := ldap.LDAPFlagSet()
	fs.AddFlagSet(ldapFS)

	remoteFilesFS, getRemoteFilesConfig := remotefiles.RemoteFilesFlagSet()
	fs.AddFlagSet(remoteFilesFS)

	s3CacheFS, getS3CacheConfig := s3cache.S3CacheFlagSet()
	fs.AddFlagSet(s3CacheFS)

	filesAddress := fs.String(
		"staticsite-files-address",
		"localhost:8147",
		"the gRPC address of the files service",
	)
	s3URL := fs.String("staticsite-s3-url", "", "S3 endpoint with optional credentials")
	s3Region := fs.String("staticsite-s3-region", "", "S3 region")
	usersInterval := fs.Duration(
		"staticsite-users-interval",
		5*time.Minute,
		"the time between two user list refreshes",
	)

	return fs, func() Config {
		return Config{
			HTTPLog: getHTTPLogConfig(),
			LDAP:    getLDAPConfig(),
			Users: UserCacheConfig{
				Interval: *usersInterval,
			},
			Files: FilesConfig{
				Address: *filesAddress,
			},
			RemoteFiles: getRemoteFilesConfig(),
			S3: S3Config{
				URL:    *s3URL,
				Region: *s3Region,
				Cache:  getS3CacheConfig(),
			},
		}
	}
}
