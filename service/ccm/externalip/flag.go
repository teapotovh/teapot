package externalip

import (
	"time"

	flag "github.com/spf13/pflag"
)

func ExternalIPFlagSet() (*flag.FlagSet, func() ExternalIPConfig) {
	fs := flag.NewFlagSet("ccm/externalip", flag.ExitOnError)

	server := fs.String("ccm-externalip-server", "https://api4.ipify.org", "the API server to request public IP from")
	retryDelay := fs.Duration("ccm-externalip-retry-dealy", 2*time.Second, "seconds to wait before retrying a failed request to the API server")
	maxRetries := fs.Uint64("ccm-externalip-max-retries", 7, "maximum number of retries to request public IP from the API server")
	interval := fs.Duration("ccm-externalip-interval", 5*time.Second, "interval between fetching the public IP address")

	return fs, func() ExternalIPConfig {
		return ExternalIPConfig{
			Server:     *server,
			RetryDelay: *retryDelay,
			MaxRetries: *maxRetries,
			Interval:   *interval,
		}
	}
}
