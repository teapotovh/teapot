package desec

import (
	"time"

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
	dryRun := fs.Bool(
		"desec-dry-run",
		false,
		"whether to run the provider in dry-run mode (requests are not sent to desec.io)",
	)
	domain := fs.String(
		"desec-domain",
		"teapot.ovh",
		"the domain to manage within desec.io",
	)
	maxRetries := fs.Int(
		"desec-max-retries",
		5,
		"maximum number of retries to perform a request against the deSEC API",
	)
	desecTimeout := fs.Duration(
		"desec-timeout",
		10*time.Second,
		"maximum amount of time alloted to a desec request",
	)
	managedTypes := fs.StringSlice(
		"desec-managed-types",
		[]string{"a", "mx", "txt"},
		"list of dns record types to be managed by the deSEC provider. This should match with external-dns's configuration",
	)

	httpLogFS, getHTTPLogConfig := httplog.HTTPLogFlagSet()
	fs.AddFlagSet(httpLogFS)

	return fs, func() DesecConfig {
		return DesecConfig{
			Token:        *token,
			Domain:       *domain,
			MaxRetries:   *maxRetries,
			DryRun:       *dryRun,
			DesecTimeout: *desecTimeout,
			ManagedTypes: *managedTypes,

			HTTPLog: getHTTPLogConfig(),
		}
	}
}
