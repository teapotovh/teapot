package net

import (
	flag "github.com/spf13/pflag"
)

func LocalFlagSet() (*flag.FlagSet, func() LocalConfig) {
	fs := flag.NewFlagSet("net/local", flag.ExitOnError)

	path := fs.String(
		"net-local-path",
		"/var/lib/teapot/net/local",
		"the path where to store state for the local machine",
	)

	return fs, func() LocalConfig {
		return LocalConfig{
			Path: *path,
		}
	}
}
