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
	"github.com/teapotovh/teapot/service/ccm"
	"github.com/teapotovh/teapot/service/ccm/externalip"
)

const (
	CodeLog            = -1
	CodeInitCCM        = -2
	CodeInitExternalIP = -3
	CodeRun            = -4
)

var (
	defaultComponents = []string{
		"externalip",
		"internalip",
	}
)

func main() {
	components := flag.StringSliceP("components", "c", defaultComponents, "list of components to run")

	fs, getCCMConfig := ccm.CCMFlagSet()
	flag.CommandLine.AddFlagSet(fs)
	fs, getLogConfig := log.LogFlagSet()
	flag.CommandLine.AddFlagSet(fs)
	fs, getExternalIPConfig := externalip.ExternalIPFlagSet()
	flag.CommandLine.AddFlagSet(fs)
	flag.Parse()

	logger, err := log.NewLogger(getLogConfig())
	if err != nil {
		// This is the only place where we use the default slog logger,
		// as our internal one has not been setup yet.
		slog.Error("error while configuring the logger", "err", err)
		os.Exit(CodeLog)
	}
	logger = logger.With("sub", "loadbalancer")
	kubelog.WithLogger(logger.With("component", "klog"))

	run := run.NewRun(run.RunConfig{Timeout: 5 * time.Second}, logger.With("component", "run"))

	ccm, err := ccm.NewCCM(getCCMConfig(), logger)
	if err != nil {
		logger.Error("error while initializing ccm controller", "err", err)
		os.Exit(CodeInitCCM)
	}

	if slices.Contains(*components, "externalip") {
		wireguard, err := externalip.NewExternalIP(ccm, getExternalIPConfig(), logger.With("component", "externalip"))
		if err != nil {
			logger.Error("error while initializing wireguard component", "err", err)
			os.Exit(CodeInitExternalIP)
		}

		run.Add("wireguard", wireguard, nil)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	run.Add("ccm", ccm, nil)

	if err := run.Run(ctx); err != nil {
		logger.Error("error while running ccm components", "err", err)
		os.Exit(CodeRun)
	}
}
