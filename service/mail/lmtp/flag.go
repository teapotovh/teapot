package lmtp

import (
	"time"

	flag "github.com/spf13/pflag"
)

func ServerFlagSet() (*flag.FlagSet, func() Config) {
	fs := flag.NewFlagSet("mail/lmtp", flag.ExitOnError)

	addr := fs.String("mail-lmtp-addr", ":24", "the address to listen on for LMTP")
	network := fs.String("mail-lmtp-network", "tcp", "the network to listen on for LMTP")
	domain := fs.String("mail-lmtp-domain", "localhost", "the LMTP server domain")
	maxMessageBytes := fs.Int64("mail-lmtp-max-message-bytes", 50*1024*1024, "the maximum accepted message size")
	readTimeout := fs.Duration("mail-lmtp-read-timeout", 5*time.Minute, "the LMTP read timeout")
	writeTimeout := fs.Duration("mail-lmtp-write-timeout", 5*time.Minute, "the LMTP write timeout")
	shutdownDelay := fs.Duration(
		"mail-lmtp-shutdown-delay",
		time.Second,
		"allowed wait time for graceful LMTP shutdown",
	)

	return fs, func() Config {
		return Config{
			Addr:            *addr,
			Network:         *network,
			Domain:          *domain,
			MaxMessageBytes: *maxMessageBytes,
			ReadTimeout:     *readTimeout,
			WriteTimeout:    *writeTimeout,
			ShutdownDelay:   *shutdownDelay,
		}
	}
}
