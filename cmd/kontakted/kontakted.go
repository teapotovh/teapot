package main

import (
	"errors"
	"log/slog"
	"net/http"
	"os"

	flag "github.com/spf13/pflag"

	"github.com/teapotovh/teapot/lib/ldap"
	"github.com/teapotovh/teapot/lib/log"
	"github.com/teapotovh/teapot/service/kontakte"
)

const (
	CodeLog    = -1
	CodeInit   = -2
	CodeListen = -3
)

func main() {
	addr := flag.String("addr", ":8080", "The address to listen on")
	jwtSecret := flag.String("jwt-secret", "", "The JWT secret key used to sign tokens")

	fs, getLdapConfig := ldap.LDAPFlagSet()
	flag.CommandLine.AddFlagSet(fs)
	fs, getLogConfig := log.LogFlagSet()
	flag.CommandLine.AddFlagSet(fs)
	flag.Parse()

	logger, err := log.NewLogger(getLogConfig())
	if err != nil {
		// This is the only place where we use the default slog logger,
		// as our internal one has not been setup yet.
		slog.Error("error while configuring the logger", "err", err)
		os.Exit(CodeLog)
	}

	jwt := *jwtSecret
	if jwt == "" {
		logger.Warn("using insecure empty jwt secret")
	}

	options := kontakte.ServerConfig{
		Addr:           *addr,
		JWTSecret:      jwt,
		FactoryOptions: getLdapConfig(),
	}

	srv, err := kontakte.NewServer(options, logger.With("sub", "kontakte"))
	if err != nil {
		logger.Error("error while initializing kontatke server", "err", err)
		os.Exit(CodeInit)
	}

	if err := srv.Listen(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("error while listening", "err", err)
		os.Exit(CodeListen)
	}
}
