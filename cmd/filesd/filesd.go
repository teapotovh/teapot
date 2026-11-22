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

	"github.com/teapotovh/teapot/lib/grpcsrv"
	"github.com/teapotovh/teapot/lib/httplog"
	"github.com/teapotovh/teapot/lib/httpsrv"
	"github.com/teapotovh/teapot/lib/log"
	"github.com/teapotovh/teapot/lib/observability"
	"github.com/teapotovh/teapot/lib/run"
	"github.com/teapotovh/teapot/service/files"
	filesgrpc "github.com/teapotovh/teapot/service/files/grpc"
	"github.com/teapotovh/teapot/service/files/web"
	"github.com/teapotovh/teapot/service/files/webdav"
)

const (
	CodeLog           = -1
	CodeObservability = -2
	CodeFiles         = -3
	CodeHTTP          = -4
	CodeGRPC          = -5
	CodeWebDav        = -6
	CodeWeb           = -7
	CodeRun           = -8
)

const (
	HTTPWebDavPrefix = "/dav"
	HTTPWebPrefix    = "/"
)

func main() {
	components := flag.StringSliceP("components", "c", []string{"grpc", "webdav", "web"}, "list of components to run")

	fs, getLogConfig := log.LogFlagSet()
	flag.CommandLine.AddFlagSet(fs)
	fs, getObservabilityConfig := observability.ObservabilityFlagSet("files")
	flag.CommandLine.AddFlagSet(fs)
	fs, getHTTPSrvConfig := httpsrv.HTTPSrvFlagSet()
	flag.CommandLine.AddFlagSet(fs)
	fs, getGRPCSrvConfig := grpcsrv.GRPCSrvFlagSet()
	flag.CommandLine.AddFlagSet(fs)
	fs, getFilesConfig := files.FilesFlagSet()
	flag.CommandLine.AddFlagSet(fs)
	fs, getWebConfig := web.WebFlagSet()
	flag.CommandLine.AddFlagSet(fs)
	flag.Parse()

	logger, err := log.NewLogger(getLogConfig())
	if err != nil {
		// This is the only place where we use the default slog logger,
		// as our internal one has not been setup yet.
		slog.Error("error while configuring the logger", "err", err) //nolint:sloglint
		os.Exit(CodeLog)
	}

	logger = httplog.WithHandler(logger)
	logger = observability.WithHandler(logger)

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

	files, err := files.NewFiles(context.Background(), getFilesConfig(), logger.With("sub", "files"))
	if err != nil {
		logger.Error("error while initiating the files subsystem", "err", err)
		os.Exit(CodeFiles)
	}

	observability.RegisterMetrics(files)
	observability.RegisterReadyz(files)
	observability.RegisterLivez(files)

	observability.RegisterMetrics(httpsrv)
	observability.RegisterReadyz(httpsrv)
	observability.RegisterLivez(httpsrv)

	grpcsrv, err := grpcsrv.NewGRPCSrv(getGRPCSrvConfig(), logger.With("sub", "grpcsrv"))
	if err != nil {
		logger.Error("error while initiating the grpcsrv subsystem", "err", err)
		os.Exit(CodeGRPC)
	}

	observability.RegisterMetrics(grpcsrv)
	observability.RegisterReadyz(grpcsrv)
	observability.RegisterLivez(grpcsrv)
	observability.RegisterTracing(grpcsrv)

	if slices.Contains(*components, "grpc") {
		grpcsrv.Register("files", filesgrpc.NewServer(files))
	}

	if slices.Contains(*components, "webdav") {
		webdav, err := webdav.NewWebDav(files, webdav.WebDavConfig{}, logger.With("sub", "webdav"))
		if err != nil {
			logger.Error("error while initiating the webdav subsystem", "err", err)
			os.Exit(CodeWebDav)
		}

		httpsrv.Register("webdav", webdav, HTTPWebDavPrefix)
		observability.RegisterMetrics(webdav)
	}

	if slices.Contains(*components, "web") {
		web, err := web.NewWeb(files, getWebConfig(), logger.With("sub", "web"))
		if err != nil {
			logger.Error("error while initiating the web subsystem", "err", err)
			os.Exit(CodeWeb)
		}

		httpsrv.Register("web", web, HTTPWebPrefix)
	}

	run.Add("files", files, nil)
	run.Add("httpsrv", httpsrv, nil)
	run.Add("grpcsrv", grpcsrv, nil)
	run.Add("observability", observability, nil)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := run.Run(ctx); err != nil {
		logger.Error("error while running files components", "err", err)
		os.Exit(CodeRun)
	}
}
