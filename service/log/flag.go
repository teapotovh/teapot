package log

import (
	flag "github.com/spf13/pflag"

	"github.com/teapotovh/teapot/lib/httphandler"
	"github.com/teapotovh/teapot/lib/httplog"
)

func LogFlagSet() (*flag.FlagSet, func() LogConfig) {
	fs := flag.NewFlagSet("log", flag.ExitOnError)

	path := fs.String("log-path", "/tmp/logd", "the path where logs are stored")

	httpHandlerFS, getHTTPHandlerConfig := httphandler.HTTPHandlerFlagSet()
	fs.AddFlagSet(httpHandlerFS)

	httpLogFS, getHTTPLogConfig := httplog.HTTPLogFlagSet()
	fs.AddFlagSet(httpLogFS)

	return fs, func() LogConfig {
		return LogConfig{
			Path: *path,

			HTTPHandler: getHTTPHandlerConfig(),
			HTTPLog:     getHTTPLogConfig(),
		}
	}
}
