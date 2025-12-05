package main

import (
	"errors"
	"log/slog"
	"net/http"
	"os"
	// "slices"

	"github.com/kataras/muxie"
	flag "github.com/spf13/pflag"

	// "github.com/teapotovh/teapot/lib/ldap"
	"github.com/teapotovh/teapot/lib/log"
	// "github.com/teapotovh/teapot/service/files"
	"github.com/teapotovh/teapot/service/files/web"
	// "github.com/teapotovh/teapot/service/files/webdav"
)

const (
	CodeLog            = -1
	CodeFilesSubsystem = -1
	CodeWebSubsystem   = -2
	CodeHTTP           = -3
)

const (
	HTTPWebDavPrefix = "/dav"
)

func main() {
	httpAddr := flag.StringP("http-addr", "h", ":8145", "http port to listen on")
	// components := flag.StringSliceP("components", "c", []string{"webdav"}, "list of components to run")

	// fs, getSessionsConfig := files.SessionsFlagSet()
	// flag.CommandLine.AddFlagSet(fs)
	// fs, getLdapConfig := ldap.LDAPFlagSet()
	// flag.CommandLine.AddFlagSet(fs)
	fs, getWebConfig := web.WebFlagSet()
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

	// config := files.FilesConfig{
	// 	SessionsConfig:    getSessionsConfig(),
	// 	LDAPFactoryConfig: getLdapConfig(),
	// }
	//
	// files, err := files.NewFiles(config, logger.With("subsystem", "files"))
	// if err != nil {
	// 	logger.Error("error while initiating files subsystem", "err", err)
	// 	os.Exit(CodeFilesSubsystem)
	// }
	//
	// // HTTP-based services
	mux := muxie.NewMux()

	// if slices.Contains(*components, "webdav") {
	// 	config := webdav.WebDavConfig{}
	// 	webdav := webdav.NewWebDav(config, logger.With("subsystem", "webdav"), files)
	//
	// 	logger.Info("registered webdav", "path", HTTPWebDavPrefix)
	// 	handler := webdav.Handler(HTTPWebDavPrefix)
	// 	mux.Handle(HTTPWebDavPrefix+"/*", handler)
	// }

	web, err := web.NewWeb(getWebConfig(), logger.With("sub", "web"))
	if err != nil {
		logger.Error("error while initiating web subsystem", "err", err)
		os.Exit(CodeWebSubsystem)
	}
	web.Register(mux)

	server := http.Server{Handler: mux, Addr: *httpAddr}
	logger.Info("listening on HTTP", "addr", *httpAddr)
	if err := server.ListenAndServe(); err != nil {
		if !errors.Is(err, http.ErrServerClosed) {
			logger.Error("error while running HTTP server", "err", err)
			os.Exit(CodeHTTP)
		}

		logger.Info("HTTP server shutdown gracefully")
	}
}
