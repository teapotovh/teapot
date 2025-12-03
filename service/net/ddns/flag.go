package ddns

import (
	"time"

	flag "github.com/spf13/pflag"
)

func CNIFlagSet() (*flag.FlagSet, func() DDNSConfig) {
	fs := flag.NewFlagSet("net/ddns", flag.ExitOnError)

	server := fs.String("net-ddns-server", "https://api4.ipify.org", "the API server to request public IP from")
	retryDelay := fs.Duration("net-ddns-retry-dealy", 2*time.Second, "seconds to wait before retrying a failed request to the API server")
	maxRetries := fs.Uint64("net-ddns-max-retries", 7, "maximum number of retries to request public IP from the API server")
	interval := fs.Duration("net-ddns-interval", 5*time.Second, "interval between fetching the public IP address")

	return fs, func() DDNSConfig {
		return DDNSConfig{
			Server:     *server,
			RetryDelay: *retryDelay,
			MaxRetries: *maxRetries,
			Interval:   *interval,
		}
	}
}
