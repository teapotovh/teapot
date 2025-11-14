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
	"github.com/teapotovh/teapot/service/net"
	"github.com/teapotovh/teapot/service/net/bgp"
	"github.com/teapotovh/teapot/service/net/wireguard"
)

const (
	CodeLog           = -1
	CodeInitNet       = -2
	CodeInitWireguard = -3
	CodeInitBGP       = -4
	CodeRun           = -5
)

func main() {
	components := flag.StringSliceP("components", "c", []string{"wireguard", "bgp"}, "list of components to run")

	fs, getNetConfig := net.NetFlagSet()
	flag.CommandLine.AddFlagSet(fs)
	fs, getLogConfig := log.LogFlagSet()
	flag.CommandLine.AddFlagSet(fs)
	fs, getWireguardConfig := wireguard.WireguardFlagSet()
	flag.CommandLine.AddFlagSet(fs)
	fs, getBGPConfig := bgp.BGPFlagSet()
	flag.CommandLine.AddFlagSet(fs)
	flag.Parse()

	logger, err := log.NewLogger(getLogConfig())
	if err != nil {
		// This is the only place where we use the default slog logger,
		// as our internal one has not been setup yet.
		slog.Error("error while configuring the logger", "err", err)
		os.Exit(CodeLog)
	}
	kubelog.WithLogger(logger.With("sub", "net"))
	logger = logger.With("sub", "net")

	run := run.NewRun(run.RunConfig{Timeout: 5 * time.Second}, logger.With("component", "run"))

	net, err := net.NewNet(getNetConfig(), logger)
	if err != nil {
		logger.Error("error while initializing net controller", "err", err)
		os.Exit(CodeInitNet)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if slices.Contains(*components, "wireguard") {
		wireguard, err := wireguard.NewWireguard(net, getWireguardConfig(), logger.With("component", "wireguard"))
		if err != nil {
			logger.Error("error while initializing wireguard component", "err", err)
			os.Exit(CodeInitWireguard)
		}

		run.Add("wireguard", wireguard, nil)
	}

	if slices.Contains(*components, "bgp") {
		bgp, err := bgp.NewBGP(net, getBGPConfig(), logger.With("component", "bgp"))
		if err != nil {
			logger.Error("error while initializing bgp component", "err", err)
			os.Exit(CodeInitBGP)
		}

		run.Add("bgp", bgp, nil)
	}

	run.Add("local", net.Local(), nil)
	run.Add("cluster", net.Cluster(), nil)
	run.Add("net", net, nil)

	if err := run.Run(ctx); err != nil {
		logger.Error("error while running net components", "err", err)
		os.Exit(CodeRun)
	}
}
