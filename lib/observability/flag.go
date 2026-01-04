package observability

import (
	"net"
	"strconv"

	flag "github.com/spf13/pflag"
)

func ObservabilityFlagSet() (*flag.FlagSet, func() ObservabilityConfig) {
	fs := flag.NewFlagSet("observability", flag.ExitOnError)

	ip := fs.IP("observability-ip", net.IPv4zero, "the address on which to open the HTTP server for observability")
	port := fs.Int16("observability-port", 8146, "the port on which to open the HTTP server for observability")

	return fs, func() ObservabilityConfig {
		return ObservabilityConfig{
			Address: net.JoinHostPort(ip.String(), strconv.Itoa(int(*port))),
		}
	}
}
