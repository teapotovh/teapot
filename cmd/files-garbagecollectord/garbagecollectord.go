package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	flag "github.com/spf13/pflag"

	liblog "github.com/teapotovh/teapot/lib/log"
	"github.com/teapotovh/teapot/lib/observability"
	"github.com/teapotovh/teapot/lib/run"
	"github.com/teapotovh/teapot/service/files/garbagecollector"
)

const (
	CodeLog              = -1
	CodeObservability    = -2
	CodeGarbageCollector = -3
	CodeRun              = -4
)

type exitAfter struct {
	runnable run.Runnable
	cancel   context.CancelFunc
}

func (e exitAfter) Run(step run.Step) error {
	defer e.cancel()

	return e.runnable.Run(step)
}

func main() {
	fs, getLogConfig := liblog.LogFlagSet()
	flag.CommandLine.AddFlagSet(fs)

	fs, getObservabilityConfig := observability.ObservabilityFlagSet("files-garbagecollector")
	flag.CommandLine.AddFlagSet(fs)

	fs, getGarbageCollectorConfig := garbagecollector.GarbageCollectorFlagSet()
	flag.CommandLine.AddFlagSet(fs)

	flag.Parse()

	logger, err := liblog.NewLogger(getLogConfig())
	if err != nil {
		// This is the only place where we use the default slog logger,
		// as our internal one has not been setup yet.
		slog.Error("error while configuring the logger", "err", err) //nolint:sloglint
		os.Exit(CodeLog)
	}

	observability, err := observability.NewObservability(
		getObservabilityConfig(),
		logger.With("sub", "observability"),
	)
	if err != nil {
		logger.Error("error while initiating the observability subsystem", "err", err)
		os.Exit(CodeObservability)
	}

	collector, err := garbagecollector.NewGarbageCollector(
		context.Background(),
		getGarbageCollectorConfig(),
		logger.With("sub", "garbagecollector"),
	)
	if err != nil {
		logger.Error("error while initiating the garbage collector", "err", err)
		os.Exit(CodeGarbageCollector)
	}

	observability.RegisterMetrics(collector)
	observability.RegisterReadyz(collector)

	runner := run.NewRun(run.RunConfig{Timeout: 5 * time.Second}, logger.With("sub", "run"))

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	runner.Add("observability", observability, nil)
	runner.Add("garbagecollector", exitAfter{runnable: collector, cancel: cancel}, nil)

	if err := runner.Run(ctx); err != nil {
		logger.Error("error while running the garbage collector", "err", err)
		os.Exit(CodeRun)
	}
}
