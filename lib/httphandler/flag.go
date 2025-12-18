package httphandler

import (
	flag "github.com/spf13/pflag"
)

func HTTPHandlerFlagSet() (*flag.FlagSet, func() HTTPHandlerConfig) {
	fs := flag.NewFlagSet("httphandler", flag.ExitOnError)

	contact := fs.String("httphandler-contact", "root@teapot.ovh", "the contact information for where to report errors")

	return fs, func() HTTPHandlerConfig {
		return HTTPHandlerConfig{
			Contact: *contact,
		}
	}
}
