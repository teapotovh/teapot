package ldapserver

import (
	"net"
	"strconv"
	"time"

	flag "github.com/spf13/pflag"
)

func LDAPSrvFlagSet() (*flag.FlagSet, func() LDAPSrvConfig) {
	fs := flag.NewFlagSet("ldapsrv", flag.ExitOnError)

	ip := fs.IP("ldapsrv-ip", net.IPv4zero, "the address on which to open the LDAP server")
	port := fs.Int16("ldapsrv-port", 1389, "the port on which to open the LDAP server")
	shutdownDelay := fs.Duration("ldapsrv-shutdown-delay", time.Second, "allowed wait time for graceful shutdown")
	readTimeout := fs.Duration("ldapsrv-read-timeout", 0, "read timeout on the TCP connection (0 to disable)")
	writeTimeout := fs.Duration("ldapsrv-write-timeout", 0, "write timeout on the TCP connection (0 to disable)")

	return fs, func() LDAPSrvConfig {
		return LDAPSrvConfig{
			Address:       net.JoinHostPort(ip.String(), strconv.Itoa(int(*port))),
			ShutdownDelay: *shutdownDelay,
			ReadTimeout:   *readTimeout,
			WriteTimeout:  *writeTimeout,
		}
	}
}
