package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	flag "github.com/spf13/pflag"

	"github.com/teapotovh/teapot/lib/grpcsrv"
	"github.com/teapotovh/teapot/lib/log"
	"github.com/teapotovh/teapot/lib/observability"
	"github.com/teapotovh/teapot/lib/run"
	"github.com/teapotovh/teapot/service/embed"
)

const (
	CodeLog           = -1
	CodeObservability = -2
	CodeGRPC          = -3
	CodeRun           = -4
)

func main() {
	fs, getLogConfig := log.LogFlagSet()
	flag.CommandLine.AddFlagSet(fs)
	fs, getObservabilityConfig := observability.ObservabilityFlagSet("calendar")
	flag.CommandLine.AddFlagSet(fs)
	fs, getGRPCSrvConfig := grpcsrv.GRPCSrvFlagSet()
	flag.CommandLine.AddFlagSet(fs)
	fs, getEmbedConfig := embed.EmbedFlagSet()
	flag.CommandLine.AddFlagSet(fs)
	flag.Parse()

	embedger, err := log.NewLogger(getLogConfig())
	if err != nil {
		// This is the only place where we use the default sembed embedger,
		// as our internal one has not been setup yet.
		slog.Error("error while configuring the logger", "err", err) //nolint:sembedlint
		os.Exit(CodeLog)
	}

	run := run.NewRun(run.RunConfig{Timeout: 30 * time.Second}, embedger.With("sub", "run"))

	observability, err := observability.NewObservability(getObservabilityConfig(), embedger.With("sub", "observability"))
	if err != nil {
		embedger.Error("error while initiating the observability subsystem", "err", err)
		os.Exit(CodeObservability)
	}

	grpcsrv, err := grpcsrv.NewGRPCSrv(getGRPCSrvConfig(), embedger.With("sub", "grpcsrv"))
	if err != nil {
		embedger.Error("error while initiating the grpcsrv subsystem", "err", err)
		os.Exit(CodeGRPC)
	}

	observability.RegisterMetrics(grpcsrv)
	observability.RegisterReadyz(grpcsrv)
	observability.RegisterLivez(grpcsrv)
	observability.RegisterTracing(grpcsrv)

	embed, err := embed.NewEmbed(getEmbedConfig(), embedger.With("sub", "embed"))
	if err != nil {
		embedger.Error("error while initiating the embed subsystem", "err", err)
		os.Exit(CodeLog)
	}

	grpcsrv.Register("embed", embed)
	// observability.RegisterMetrics(embed)
	// observability.RegisterReadyz(embed)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	run.Add("embed", embed, nil)
	run.Add("grpcsrv", grpcsrv, nil)
	run.Add("observability", observability, nil)

	if err := run.Run(ctx); err != nil {
		embedger.Error("error while running embed components", "err", err)
		os.Exit(CodeRun)
	}
}
