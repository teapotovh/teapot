package httpsrv

import (
	"net"
	"strconv"
	"time"

	flag "github.com/spf13/pflag"
)

func HTTPSrvFlagSet() (*flag.FlagSet, func() HTTPSrvConfig) {
	fs := flag.NewFlagSet("httpsrv", flag.ExitOnError)

	ip := fs.IP("httpaddr-ip", net.IPv4zero, "the address on which to open the HTTP server")
	port := fs.Int16("httpaddr-port", 8145, "the port on which to open the HTTP server")
	shutdownDelay := fs.Duration("httpaddr-shutdown-delay", time.Second, "allowed wait time for graceful shutdown")

	return fs, func() HTTPSrvConfig {
		return HTTPSrvConfig{
			Address:       net.JoinHostPort(ip.String(), strconv.Itoa(int(*port))),
			ShutdownDelay: *shutdownDelay,
		}
	}
}
