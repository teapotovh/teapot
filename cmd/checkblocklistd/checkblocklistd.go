package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	flag "github.com/spf13/pflag"

	"github.com/teapotovh/teapot/lib/log"
	"github.com/teapotovh/teapot/lib/observability"
	"github.com/teapotovh/teapot/lib/run"
	"github.com/teapotovh/teapot/service/checkblocklist"
)

const (
	CodeLog            = -1
	CodeObservability  = -2
	CodeCheckBlockList = -3
	CodeRun            = -4
)

func main() {
	fs, getLogConfig := log.LogFlagSet()
	flag.CommandLine.AddFlagSet(fs)
	fs, getObservabilityConfig := observability.ObservabilityFlagSet()
	flag.CommandLine.AddFlagSet(fs)
	fs, getCheckBlockListConfig := checkblocklist.CheckBlockListFlagSet()
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

	checkblocklist, err := checkblocklist.NewCheckBlockList(getCheckBlockListConfig(), logger.With("sub", "checkblocklist"))
	if err != nil {
		logger.Error("error while initiating the checkblocklist subsystem", "err", err)
		os.Exit(CodeCheckBlockList)
	}

	observability.RegisterMetrics(checkblocklist)
	observability.RegisterReadyz(checkblocklist)
	observability.RegisterLivez(checkblocklist)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	run.Add("observability", observability, nil)
	run.Add("checkblocklist", checkblocklist, nil)

	if err := run.Run(ctx); err != nil {
		logger.Error("error while running checkblocklist components", "err", err)
		os.Exit(CodeRun)
	}
}
