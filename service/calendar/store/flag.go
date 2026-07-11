package store

import (
	"time"

	flag "github.com/spf13/pflag"
)

func StoreFlagSet() (*flag.FlagSet, func() StoreConfig) {
	fs := flag.NewFlagSet("calendar/store", flag.ExitOnError)

	timeout := fs.Duration("calendar-store-timeout", time.Minute, "timeout for store connection setup")
	typ := fs.String("calendar-store-type", "mem", "the type of store to back the calendar server. Options: mem, psql")
	url := fs.String(
		"calendar-store-url",
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
