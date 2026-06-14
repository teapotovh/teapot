package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"slices"
	"syscall"
	"time"

	flag "github.com/spf13/pflag"

	"github.com/teapotovh/teapot/lib/kubelog"
	"github.com/teapotovh/teapot/lib/log"
	"github.com/teapotovh/teapot/lib/run"
	"github.com/teapotovh/teapot/service/loadbalancer"
)

const (
	CodeLog              = -1
	CodeInitLoadBalancer = -2
	CodeRun              = -4
)

var defaultComponents = []string{
	"arp",
}

func main() {
	components := flag.StringSliceP("components", "c", defaultComponents, "list of components to run")

	fs, getLoadBalancerConfig := loadbalancer.LoadBalancerFlagSet()
	flag.CommandLine.AddFlagSet(fs)
	fs, getLogConfig := log.LogFlagSet()
	flag.CommandLine.AddFlagSet(fs)
	flag.Parse()

	logger, err := log.NewLogger(getLogConfig())
	if err != nil {
		// This is the only place where we use the default slog logger,
		// as our internal one has not been setup yet.
		slog.Error("error while configuring the logger", "err", err) //nolint:sloglint
		os.Exit(CodeLog)
	}

	kubelog.WithLogger(logger.With("sub", "klog"))

	run := run.NewRun(run.RunConfig{Timeout: 5 * time.Second}, logger.With("sub", "run"))

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if slices.Contains(*components, "loadbalancer") {
		lb, err := loadbalancer.NewLoadBalancer(getLoadBalancerConfig(), logger.With("sub", "loadbalancer"))
		if err != nil {
			logger.Error("error while initializing loadbalancer controller", "err", err)
			os.Exit(CodeInitLoadBalancer)
		}

		run.Add("loadbalancer", lb, nil)
	}

	if err := run.Run(ctx); err != nil {
		logger.Error("error while running loadbalancer components", "err", err)
		os.Exit(CodeRun)
	}
}
