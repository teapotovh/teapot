package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	flag "github.com/spf13/pflag"

	"github.com/teapotovh/teapot/lib/ldapsrv"
	"github.com/teapotovh/teapot/lib/log"
	"github.com/teapotovh/teapot/lib/observability"
	"github.com/teapotovh/teapot/lib/run"
	"github.com/teapotovh/teapot/service/bottin"
)

const (
	CodeLog           = -1
	CodeObservability = -2
	CodeLDAP          = -3
	CodeBottin        = -4
	CodeInitialize    = -5
	CodeRun           = -6
)

func main() {
	fs, getLogConfig := log.LogFlagSet()
	flag.CommandLine.AddFlagSet(fs)
	fs, getObservabilityConfig := observability.ObservabilityFlagSet()
	flag.CommandLine.AddFlagSet(fs)
	fs, getLDAPSrvConfig := ldapsrv.LDAPSrvFlagSet()
	flag.CommandLine.AddFlagSet(fs)
	fs, getBottinConfig := bottin.BottinFlagSet()
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

	ldapsrv, err := ldapsrv.NewServer(getLDAPSrvConfig(), logger.With("sub", "ldapsrv"))
	if err != nil {
		logger.Error("error while initiating the ldapsrv subsystem", "err", err)
		os.Exit(CodeLDAP)
	}

	bottin, err := bottin.NewBottin(getBottinConfig(), logger.With("sub", "bottin"))
	if err != nil {
		logger.Error("error while constructing server", "err", err)
		os.Exit(CodeBottin)
	}

	ldapsrv.Register(bottin)

	// observability.RegisterMetrics(bottin)
	// observability.RegisterReadyz(bottin)

	observability.RegisterMetrics(ldapsrv)
	// observability.RegisterReadyz(httpsrv)
	// observability.RegisterLivez(httpsrv)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := bottin.Initialize(ctx); err != nil {
		logger.Error("error while running initializing bottin", "err", err)
		os.Exit(CodeInitialize)
	}

	run.Add("ldapsrv", ldapsrv, nil)
	run.Add("observability", observability, nil)

	if err := run.Run(ctx); err != nil {
		logger.Error("error while running bottin components", "err", err)
		os.Exit(CodeRun)
	}
}
