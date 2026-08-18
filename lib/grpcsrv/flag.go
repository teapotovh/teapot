package grpcsrv

import (
	"net"
	"strconv"
	"time"

	flag "github.com/spf13/pflag"
)

func GRPCSrvFlagSet() (*flag.FlagSet, func() GRPCSrvConfig) {
	fs := flag.NewFlagSet("grpcsrv", flag.ExitOnError)

	ip := fs.IP("grpcsrv-ip", net.IPv4zero, "the address on which to open the gRPC server")
	port := fs.Int16("grpcsrv-port", 8147, "the port on which to open the gRPC server")
	shutdownDelay := fs.Duration(
		"grpcsrv-shutdown-delay",
		time.Second,
		"allowed wait time for graceful shutdown of the gRPC server",
	)

	return fs, func() GRPCSrvConfig {
		return GRPCSrvConfig{
			Address:       net.JoinHostPort(ip.String(), strconv.Itoa(int(*port))),
			ShutdownDelay: *shutdownDelay,
		}
	}
}
