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
	"github.com/teapotovh/teapot/lib/run"
	"github.com/teapotovh/teapot/service/files"
	"github.com/teapotovh/teapot/service/files/web"
	"github.com/teapotovh/teapot/service/files/webdav"
)

const (
	CodeLog    = -1
	CodeFiles  = -1
	CodeWebDav = -2
	CodeWeb    = -3
	CodeHTTP   = -4
	CodeRun    = -5
)

const (
	HTTPWebDavPrefix = "/dav"
	HTTPWebPrefix    = "/"
)

func main() {
	components := flag.StringSliceP("components", "c", []string{"webdav", "web"}, "list of components to run")

	fs, getFilesConfig := files.FilesFlagSet()
	flag.CommandLine.AddFlagSet(fs)
	fs, getHTTPSrvConfig := httpsrv.HTTPSrvFlagSet()
	flag.CommandLine.AddFlagSet(fs)
	fs, getWebConfig := web.WebFlagSet()
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

	run := run.NewRun(run.RunConfig{Timeout: 5 * time.Second}, logger.With("sub", "run"))

	files, err := files.NewFiles(getFilesConfig(), logger.With("sub", "files"))
	if err != nil {
		logger.Error("error while initiating the files subsystem", "err", err)
		os.Exit(CodeFiles)
	}

	httpsrv, err := httpsrv.NewHTTPSrv(getHTTPSrvConfig(), logger.With("sub", "httpsrv"))
	if err != nil {
		logger.Error("error while initiating the httpsrv subsystem", "err", err)
		os.Exit(CodeWeb)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if slices.Contains(*components, "webdav") {
		webdav, err := webdav.NewWebDav(files, webdav.WebDavConfig{}, logger.With("sub", "webdav"))
		if err != nil {
			logger.Error("error while initiating the webdav subsystem", "err", err)
			os.Exit(CodeWeb)
		}

		httpsrv.Register("webdav", webdav, HTTPWebDavPrefix)
	}

	if slices.Contains(*components, "web") {
		web, err := web.NewWeb(files, getWebConfig(), logger.With("sub", "web"))
		if err != nil {
			logger.Error("error while initiating the web subsystem", "err", err)
			os.Exit(CodeWeb)
		}

		httpsrv.Register("web", web, HTTPWebPrefix)
	}

	run.Add("httpsrv", httpsrv, nil)

	if err := run.Run(ctx); err != nil {
		logger.Error("error while running net components", "err", err)
		os.Exit(CodeRun)
	}
}
