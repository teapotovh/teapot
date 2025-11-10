package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	flag "github.com/spf13/pflag"

	"github.com/teapotovh/teapot/lib/kubelog"
	"github.com/teapotovh/teapot/lib/log"
	"github.com/teapotovh/teapot/service/net"
)

const (
	CodeLog    = -1
	CodeInit   = -2
	CodeListen = -3
)

func main() {
	fs, getNetConfig := net.NetFlagSet()
	flag.CommandLine.AddFlagSet(fs)
	fs, getLogConfig := log.LogFlagSet()
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

	net, err := net.NewNet(getNetConfig(), logger.With("sub", "net"))
	if err != nil {
		logger.Error("error while initializing net controller", "err", err)
		os.Exit(CodeInit)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := net.Run(ctx); err != nil {
		logger.Error("error while running net controller", "err", err)
		os.Exit(CodeListen)
	}
}
