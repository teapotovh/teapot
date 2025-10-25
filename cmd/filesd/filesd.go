package main

import (
	"errors"
	"log/slog"
	"net/http"
	"os"
	"slices"

	"github.com/kataras/muxie"
	flag "github.com/spf13/pflag"

	"github.com/teapotovh/teapot/lib/ldap"
	"github.com/teapotovh/teapot/service/files"
	"github.com/teapotovh/teapot/service/files/webdav"
)

const (
	CodeFilesSubsystem = -1
	CodeHTTP           = -2
)

const (
	HTTPWebDavPrefix = "/dav/"
)

func main() {
	httpAddr := flag.StringP("http-addr", "h", ":8145", "http port to listen on")
	components := flag.StringSliceP("components", "c", []string{"webdav"}, "list of components to run")

	fs, getSessionsConfig := files.SessionsFlagSet()
	flag.CommandLine.AddFlagSet(fs)
	fs, getLdapConfig := ldap.LDAPFlagSet()
	flag.CommandLine.AddFlagSet(fs)

	flag.Parse()

	config := files.FilesConfig{
		SessionsConfig:    getSessionsConfig(),
		LDAPFactoryConfig: getLdapConfig(),
	}

	files, err := files.NewFiles(config, slog.With("subsystem", "files"))
	if err != nil {
		slog.Error("error while initiating files subsystem", "err", err)
		os.Exit(CodeFilesSubsystem)
	}

	// HTTP-based services
	mux := muxie.NewMux()

	if slices.Contains(*components, "webdav") {
		config := webdav.WebDavConfig{}
		webdav := webdav.NewWebDav(config, slog.With("subsystem", "webdav"), files)

		slog.Info("registered webdav", "path", HTTPWebDavPrefix)
		handler := webdav.Handler(HTTPWebDavPrefix)
		mux.Handle(HTTPWebDavPrefix+"*", handler)
	}

	slog.Info("listening on HTTP", "addr", *httpAddr)
	server := http.Server{Handler: mux, Addr: *httpAddr}
	if err := server.ListenAndServe(); err != nil {
		if !errors.Is(err, http.ErrServerClosed) {
			slog.Error("error while running HTTP server", "err", err)
			os.Exit(CodeHTTP)
		}

		slog.Info("HTTP server shutdown gracefully")
	}
}
