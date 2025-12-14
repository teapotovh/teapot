package httpsrv

import (
	"fmt"
	"net"
	"time"

	flag "github.com/spf13/pflag"
)

func HttpSrvFlagSet() (*flag.FlagSet, func() HttpSrvConfig) {
	fs := flag.NewFlagSet("httpsrv", flag.ExitOnError)

	ip := fs.IP("httpaddr-ip", net.IPv4zero, "the address on which to open the HTTP server")
	port := fs.Int16("httpaddr-port", 8145, "the port on which to open the HTTP server")
	shutdownDelay := fs.Duration("httpaddr-shutdown-delay", time.Second, "allowed wait time for graceful shutdown")

	return fs, func() HttpSrvConfig {
		return HttpSrvConfig{
			Address:       fmt.Sprintf("%s:%d", ip, *port),
			ShutdownDelay: *shutdownDelay,
		}
	}
}
