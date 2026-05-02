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
	liblog "github.com/teapotovh/teapot/lib/log"
	"github.com/teapotovh/teapot/lib/observability"
	"github.com/teapotovh/teapot/lib/run"
	"github.com/teapotovh/teapot/service/log"
)

const (
	CodeLog           = -1
	CodeObservability = -2
	CodeHTTP          = -3
	CodeRun           = -4
)

func main() {
	fs, getLogConfig := log.LogFlagSet()
	flag.CommandLine.AddFlagSet(fs)
	fs, getLibLogConfig := liblog.LogFlagSet()
	flag.CommandLine.AddFlagSet(fs)
	fs, getHTTPSrvConfig := httpsrv.HTTPSrvFlagSet()
	flag.CommandLine.AddFlagSet(fs)
	fs, getObservabilityConfig := observability.ObservabilityFlagSet()
	flag.CommandLine.AddFlagSet(fs)
	flag.Parse()

	logger, err := liblog.NewLogger(getLibLogConfig())
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

	observability.RegisterMetrics(httpsrv)
	observability.RegisterReadyz(httpsrv)
	observability.RegisterLivez(httpsrv)

	log, err := log.NewLog(getLogConfig(), logger.With("sub", "log"))
	if err != nil {
		logger.Error("error while initiating the log subsystem", "err", err)
		os.Exit(CodeLog)
	}

	httpsrv.Register("log", log, "/")
	observability.RegisterMetrics(log)
	observability.RegisterReadyz(log)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	run.Add("httpsrv", httpsrv, nil)
	run.Add("observability", observability, nil)
	run.Add("log", log, nil)

	if err := run.Run(ctx); err != nil {
		logger.Error("error while running log components", "err", err)
		os.Exit(CodeRun)
	}
}
