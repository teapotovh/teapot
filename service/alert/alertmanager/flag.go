package alertmanager

import (
	flag "github.com/spf13/pflag"

	"github.com/teapotovh/teapot/lib/httphandler"
	"github.com/teapotovh/teapot/lib/httplog"
)

func AlertManagerFlagSet() (*flag.FlagSet, func() AlertManagerConfig) {
	fs := flag.NewFlagSet("alert/alertmanager", flag.ExitOnError)

	httpLogFS, getHTTPLogConfig := httplog.HTTPLogFlagSet()
	fs.AddFlagSet(httpLogFS)

	httpHandlerFS, getHTTPHandlerConfig := httphandler.HTTPHandlerFlagSet()
	fs.AddFlagSet(httpHandlerFS)

	return fs, func() AlertManagerConfig {
		return AlertManagerConfig{
			HTTPLog:     getHTTPLogConfig(),
			HTTPHandler: getHTTPHandlerConfig(),
		}
	}
}
