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
	"github.com/teapotovh/teapot/lib/run"
	"github.com/teapotovh/teapot/service/bottin"
)

const (
	CodeLog        = -1
	CodeBottin     = -2
	CodeInitialize = -3
	CodeRun        = -4
)

func main() {
	fs, getLogConfig := log.LogFlagSet()
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

	ldapsrv := ldapsrv.NewServer(getLDAPSrvConfig(), logger.With("sub", "ldapsrv"))

	bottin, err := bottin.NewBottin(getBottinConfig(), logger.With("sub", "bottin"))
	if err != nil {
		logger.Error("error while constructing server", "err", err)
		os.Exit(CodeBottin)
	}
	ldapsrv.Handle(bottin.Routes)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := bottin.Initialize(ctx); err != nil {
		logger.Error("error while running initializing bottin", "err", err)
		os.Exit(CodeInitialize)
	}

	run.Add("ldapsrv", ldapsrv, nil)

	if err := run.Run(ctx); err != nil {
		logger.Error("error while running net components", "err", err)
		os.Exit(CodeRun)
	}
}
