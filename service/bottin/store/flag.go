package store

import (
	"time"

	flag "github.com/spf13/pflag"
)

func StoreFlagSet() (*flag.FlagSet, func() StoreConfig) {
	fs := flag.NewFlagSet("bottin/store", flag.ExitOnError)

	timeout := fs.Duration("bottin-store-timeout", time.Minute, "timeout for store connection setup")
	typ := fs.String("bottin-store-type", "mem", "the type of store to back the LDAP server. Options: mem, psql")
	url := fs.String(
		"bottin-store-url",
		"",
		"the URL connection string to connect to the store. mandatory for the psql backend",
	)

	return fs, func() StoreConfig {
		return StoreConfig{
			Timeout: *timeout,
			Type:    *typ,
			URL:     *url,
		}
	}
}
