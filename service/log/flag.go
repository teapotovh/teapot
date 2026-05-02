package log

import (
	flag "github.com/spf13/pflag"

	"github.com/teapotovh/teapot/lib/httphandler"
	"github.com/teapotovh/teapot/lib/httplog"
)

func LogFlagSet() (*flag.FlagSet, func() LogConfig) {
	fs := flag.NewFlagSet("log", flag.ExitOnError)

	path := fs.String("log-path", "/tmp/logd", "the path where logs are stored")
	capacity := fs.Uint32("log-queue-capacity", 1024, "the maximum number of logs to be processed in the queue (per source)")

	httpHandlerFS, getHTTPHandlerConfig := httphandler.HTTPHandlerFlagSet()
	fs.AddFlagSet(httpHandlerFS)

	httpLogFS, getHTTPLogConfig := httplog.HTTPLogFlagSet()
	fs.AddFlagSet(httpLogFS)

	return fs, func() LogConfig {
		return LogConfig{
			Path:     *path,
			Capacity: *capacity,

			HTTPHandler: getHTTPHandlerConfig(),
			HTTPLog:     getHTTPLogConfig(),
		}
	}
}
