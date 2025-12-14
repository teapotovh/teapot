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
	"github.com/teapotovh/teapot/service/ccm/initialize"
	"github.com/teapotovh/teapot/service/ccm/internalip"
)

const (
	CodeLog            = -1
	CodeInitCCM        = -2
	CodeInitExternalIP = -3
	CodeInitInternalIP = -4
	CodeInitInitialize = -5
	CodeRun            = -6
)

var defaultComponents = []string{
	"externalip",
	"internalip",
	"initilize",
}

func main() {
	components := flag.StringSliceP("components", "c", defaultComponents, "list of components to run")

	fs, getCCMConfig := ccm.CCMFlagSet()
	flag.CommandLine.AddFlagSet(fs)
	fs, getLogConfig := log.LogFlagSet()
	flag.CommandLine.AddFlagSet(fs)
	fs, getExternalIPConfig := externalip.ExternalIPFlagSet()
	flag.CommandLine.AddFlagSet(fs)
	fs, getInternalIPConfig := internalip.InternalIPFlagSet()
	flag.CommandLine.AddFlagSet(fs)
	flag.Parse()

	logger, err := log.NewLogger(getLogConfig())
	if err != nil {
		// This is the only place where we use the default slog logger,
		// as our internal one has not been setup yet.
		slog.Error("error while configuring the logger", "err", err)
		os.Exit(CodeLog)
	}
	kubelog.WithLogger(logger.With("sub", "klog"))

	run := run.NewRun(run.RunConfig{Timeout: 5 * time.Second}, logger.With("sub", "run"))

	ccm, err := ccm.NewCCM(getCCMConfig(), logger.With("sub", "ccm"))
	if err != nil {
		logger.Error("error while initializing ccm controller", "err", err)
		os.Exit(CodeInitCCM)
	}

	if slices.Contains(*components, "externalip") {
		externalip, err := externalip.NewExternalIP(ccm, getExternalIPConfig(), logger.With("sub", "externalip"))
		if err != nil {
			logger.Error("error while initializing externalip component", "err", err)
			os.Exit(CodeInitExternalIP)
		}

		run.Add("externalip", externalip, nil)
	}

	if slices.Contains(*components, "internalip") {
		internalip, err := internalip.NewInternalIP(ccm, getInternalIPConfig(), logger.With("sub", "internalip"))
		if err != nil {
			logger.Error("error while initializing internalip component", "err", err)
			os.Exit(CodeInitInternalIP)
		}

		run.Add("internalip", internalip, nil)
	}

	if slices.Contains(*components, "initilize") {
		initialize, err := initialize.NewInitialize(ccm, logger.With("sub", "initialize"))
		if err != nil {
			logger.Error("error while initializing initialize component", "err", err)
			os.Exit(CodeInitInitialize)
		}

		run.Add("initialize", initialize, nil)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	run.Add("ccm", ccm, nil)

	if err := run.Run(ctx); err != nil {
		logger.Error("error while running ccm components", "err", err)
		os.Exit(CodeRun)
	}
}
