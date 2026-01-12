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

	"github.com/teapotovh/teapot/lib/httpsrv"
	"github.com/teapotovh/teapot/lib/log"
	"github.com/teapotovh/teapot/lib/observability"
	"github.com/teapotovh/teapot/lib/run"
	"github.com/teapotovh/teapot/service/alert"
	"github.com/teapotovh/teapot/service/alert/alertmanager"
)

const (
	CodeLog           = -1
	CodeObservability = -2
	CodeAlert         = -3
	CodeHTTP          = -4
	CodeAlertManager  = -5
	CodeRun           = -6
)

const (
	HTTPAlertManagerPrefix = "/alertmanager"
)

func main() {
	components := flag.StringSliceP("components", "c", []string{"alertmanager"}, "list of components to run")

	fs, getLogConfig := log.LogFlagSet()
	flag.CommandLine.AddFlagSet(fs)
	fs, getObservabilityConfig := observability.ObservabilityFlagSet()
	flag.CommandLine.AddFlagSet(fs)
	fs, getHTTPSrvConfig := httpsrv.HTTPSrvFlagSet()
	flag.CommandLine.AddFlagSet(fs)
	fs, getAlertConfig := alert.AlertFlagSet()
	flag.CommandLine.AddFlagSet(fs)
	fs, getAlertManagerConfig := alertmanager.AlertManagerFlagSet()
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

	alert, err := alert.NewAlert(getAlertConfig(), logger.With("sub", "alert"))
	if err != nil {
		logger.Error("error while initiating the alert subsystem", "err", err)
		os.Exit(CodeAlert)
	}

	observability.RegisterMetrics(alert)
	observability.RegisterReadyz(alert)

	httpsrv, err := httpsrv.NewHTTPSrv(getHTTPSrvConfig(), logger.With("sub", "httpsrv"))
	if err != nil {
		logger.Error("error while initiating the httpsrv subsystem", "err", err)
		os.Exit(CodeHTTP)
	}

	observability.RegisterMetrics(httpsrv)
	observability.RegisterReadyz(httpsrv)
	observability.RegisterLivez(httpsrv)

	if slices.Contains(*components, "alertmanager") {
		alertmanager, err := alertmanager.NewAlertManager(
			alert,
			getAlertManagerConfig(),
			logger.With("sub", "alertmanager"),
		)
		if err != nil {
			logger.Error("error while initiating the httpsrv subsystem", "err", err)
			os.Exit(CodeAlertManager)
		}

		httpsrv.Register("alertmanager", alertmanager, HTTPAlertManagerPrefix)
		observability.RegisterMetrics(alertmanager)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	run.Add("httpsrv", httpsrv, nil)
	run.Add("observability", observability, nil)
	run.Add("alert", alert, nil)

	if err := run.Run(ctx); err != nil {
		logger.Error("error while running net components", "err", err)
		os.Exit(CodeRun)
	}
}
