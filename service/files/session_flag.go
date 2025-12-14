package files

import (
	"time"

	flag "github.com/spf13/pflag"
)

func SessionsFlagSet() (*flag.FlagSet, func() SessionsConfig) {
	fs := flag.NewFlagSet("files/sessions", flag.ExitOnError)

	cacheSize := fs.Int("files-session-cache-size", 128, "how many sessions to cache in memory")
	cacheLifetime := fs.Duration("files-session-cache-lifetime", time.Hour*4, "how long to cache sessions")

	return fs, func() SessionsConfig {
		return SessionsConfig{
			CacheSize:     *cacheSize,
			CacheLifetime: *cacheLifetime,
		}
	}
}
