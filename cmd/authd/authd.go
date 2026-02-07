package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	flag "github.com/spf13/pflag"

	"github.com/teapotovh/teapot/lib/httpsrv"
	"github.com/teapotovh/teapot/lib/log"
	"github.com/teapotovh/teapot/lib/observability"
	"github.com/teapotovh/teapot/lib/run"
	"github.com/teapotovh/teapot/service/auth"
)

const (
	CodeLog           = -1
	CodeObservability = -2
	CodeAuth          = -3
	CodeHTTP          = -4
	CodeRun           = -5
)

const HTTPAuthPrefix = "/"

func main() {
	fs, getLogConfig := log.LogFlagSet()
	flag.CommandLine.AddFlagSet(fs)
	fs, getObservabilityConfig := observability.ObservabilityFlagSet()
	flag.CommandLine.AddFlagSet(fs)
	fs, getHTTPSrvConfig := httpsrv.HTTPSrvFlagSet()
	flag.CommandLine.AddFlagSet(fs)
	fs, getAuthConfig := auth.AuthFlagSet()
	flag.CommandLine.AddFlagSet(fs)
	flag.Parse()

	logger, err := log.NewLogger(getLogConfig())
	if err != nil {
		// This is the only place where we use the default slog logger,
		// as our internal one has not been setup yet.
		slog.Error("error while configuring the logger", "err", err) //nolint:sloglint
		os.Exit(CodeLog)
	}

	run := run.NewRun(run.RunConfig{Timeout: 5 * time.Second}, logger.With("sub", "run"))

	observability, err := observability.NewObservability(getObservabilityConfig(), logger.With("sub", "observability"))
	if err != nil {
		logger.Error("error while initiating the observability subsystem", "err", err)
		os.Exit(CodeObservability)
	}

	httpsrv, err := httpsrv.NewHTTPSrv(getHTTPSrvConfig(), logger.With("sub", "httpsrv"))
	if err != nil {
		logger.Error("error while initiating the httpsrv subsystem", "err", err)
		os.Exit(CodeHTTP)
	}

	auth, err := auth.NewAuth(getAuthConfig(), logger.With("sub", "auth"))
	if err != nil {
		logger.Error("error while initializing the auth subsystem", "err", err)
		os.Exit(CodeAuth)
	}

	httpsrv.Register("auth", auth, HTTPAuthPrefix)
	// TODO
	// observability.RegisterMetrics(auth)
	// observability.RegisterReadyz(auth)

	observability.RegisterMetrics(httpsrv)
	observability.RegisterReadyz(httpsrv)
	observability.RegisterLivez(httpsrv)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	run.Add("httpsrv", httpsrv, nil)
	run.Add("observability", observability, nil)

	if err := run.Run(ctx); err != nil {
		logger.Error("error while running auth components", "err", err)
		os.Exit(CodeRun)
	}
}
