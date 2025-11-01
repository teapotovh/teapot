package store

import (
	flag "github.com/spf13/pflag"
)

func StoreFlagSet() (*flag.FlagSet, func() StoreConfig) {
	fs := flag.NewFlagSet("bottin/store", flag.ExitOnError)

	typ := fs.String("bottin-store-type", "mem", "the type of store to back the LDAP server. Options: mem, psql")
	url := fs.String("bottin-store-url", "", "the URL connection string to connect to the store. mandatory for the psql backend")

	return fs, func() StoreConfig {
		return StoreConfig{
			Type: *typ,
			URL:  *url,
		}
	}
}
