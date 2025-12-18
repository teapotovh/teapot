package web

import (
	flag "github.com/spf13/pflag"

	"github.com/teapotovh/teapot/lib/httplog"
	"github.com/teapotovh/teapot/lib/webauth"
	"github.com/teapotovh/teapot/lib/webhandler"
)

func WebFlagSet() (*flag.FlagSet, func() WebConfig) {
	fs := flag.NewFlagSet("files/web", flag.ExitOnError)

	httpLogFS, getHTTPLogConfig := httplog.HTTPLogFlagSet()
	fs.AddFlagSet(httpLogFS)

	webHandlerFS, getWebHandlerConfig := webhandler.WebHandlerFlagSet()
	fs.AddFlagSet(webHandlerFS)

	webAuthFS, getWebAuthConfig := webauth.WebAuthFlagSet("files/web")
	fs.AddFlagSet(webAuthFS)

	return fs, func() WebConfig {
		return WebConfig{
			HTTPLog:    getHTTPLogConfig(),
			WebHandler: getWebHandlerConfig(),
			WebAuth:    getWebAuthConfig(),
		}
	}
}
