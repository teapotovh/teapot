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
	"github.com/teapotovh/teapot/service/calendar"
)

const (
	CodeLog           = -1
	CodeObservability = -2
	CodeHTTP          = -3
	CodeCalendar      = -4
	CodeRun           = -5
)

const (
	HTTPCalendarPrefix = "/"
)

func main() {
	fs, getLogConfig := log.LogFlagSet()
	flag.CommandLine.AddFlagSet(fs)
	fs, getObservabilityConfig := observability.ObservabilityFlagSet("calendar")
	flag.CommandLine.AddFlagSet(fs)
	fs, getHTTPSrvConfig := httpsrv.HTTPSrvFlagSet()
	flag.CommandLine.AddFlagSet(fs)
	fs, getCalendarConfig := calendar.CalendarFlagSet()
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

	calendar, err := calendar.NewCalendar(getCalendarConfig(), logger.With("sub", "calendar"))
	if err != nil {
		logger.Error("error while initiating the calendar subsystem", "err", err)
		os.Exit(CodeCalendar)
	}

	observability.RegisterMetrics(calendar)
	observability.RegisterReadyz(calendar)
	observability.RegisterTracing(calendar)

	observability.RegisterTracing(calendar.Store())

	observability.RegisterMetrics(httpsrv)
	observability.RegisterReadyz(httpsrv)
	observability.RegisterLivez(httpsrv)

	httpsrv.Register("calendar", calendar, HTTPCalendarPrefix)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	run.Add("calendar/store", calendar.Store(), nil)
	run.Add("httpsrv", httpsrv, nil)
	run.Add("observability", observability, nil)

	if err := run.Run(ctx); err != nil {
		logger.Error("error while running calendar components", "err", err)
		os.Exit(CodeRun)
	}
}
