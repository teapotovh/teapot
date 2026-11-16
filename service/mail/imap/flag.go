package imap

import (
	"crypto/tls"
	"errors"
	"fmt"
	"time"

	flag "github.com/spf13/pflag"
)

func ServerFlagSet() (*flag.FlagSet, func() (ServerConfig, error)) {
	fs := flag.NewFlagSet("mail/imap", flag.ExitOnError)

	addr := fs.String("mail-imap-addr", ":143", "the address to listen on for IMAP")
	network := fs.String("mail-imap-network", "tcp", "the network to listen on for IMAP")
	insecureAuth := fs.Bool("mail-imap-insecure-auth", false, "allow authentication without TLS")
	certFile := fs.String("mail-imap-tls-cert", "", "the TLS certificate file")
	keyFile := fs.String("mail-imap-tls-key", "", "the TLS key file")
	pollInterval := fs.Duration(
		"mail-imap-poll-interval",
		10*time.Second,
		"the fallback poll interval for IMAP changes",
	)
	shutdownDelay := fs.Duration(
		"mail-imap-shutdown-delay",
		time.Second,
		"allowed wait time for graceful IMAP shutdown",
	)

	return fs, func() (ServerConfig, error) {
		config := ServerConfig{
			Addr:          *addr,
			Network:       *network,
			InsecureAuth:  *insecureAuth,
			PollInterval:  *pollInterval,
			ShutdownDelay: *shutdownDelay,
		}

		if (*certFile == "") != (*keyFile == "") {
			return ServerConfig{}, ErrTLSKeyPair
		}

		if *certFile != "" {
			cert, err := tls.LoadX509KeyPair(*certFile, *keyFile)
			if err != nil {
				return ServerConfig{}, fmt.Errorf("error while loading IMAP TLS key pair: %w", err)
			}

			config.TLS = &tls.Config{
				Certificates: []tls.Certificate{cert},
				MinVersion:   tls.VersionTLS12,
			}
		}

		return config, nil
	}
}

// ErrTLSKeyPair is returned when only one part of a TLS key pair is set.
var ErrTLSKeyPair = errors.New("both IMAP TLS certificate and key must be set")
