package desec

import (
	flag "github.com/spf13/pflag"
	"github.com/teapotovh/teapot/lib/httplog"
)

func DesecFlagSet() (*flag.FlagSet, func() DesecConfig) {
	fs := flag.NewFlagSet("desec", flag.ExitOnError)

	token := fs.String(
		"desec-token",
		"",
		"the token to access the GitHub API. Needs permission to create issues",
	)
	domain := fs.String(
		"desec-domain",
		"teapot.ovh",
		"the domain to manage within desec.io",
	)
	httpLogFS, getHTTPLogConfig := httplog.HTTPLogFlagSet()
	fs.AddFlagSet(httpLogFS)

	return fs, func() DesecConfig {
		return DesecConfig{
			Token:  *token,
			Domain: *domain,

			HTTPLog: getHTTPLogConfig(),
		}
	}
}
