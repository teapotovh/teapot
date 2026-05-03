package log

import (
	"time"

	flag "github.com/spf13/pflag"

	"github.com/teapotovh/teapot/lib/httphandler"
	"github.com/teapotovh/teapot/lib/httplog"
)

func LogFlagSet() (*flag.FlagSet, func() LogConfig) {
	fs := flag.NewFlagSet("log", flag.ExitOnError)

	path := fs.String("log-path", "/tmp/logd", "the path where logs are stored")
	capacity := fs.Uint32(
		"log-queue-capacity",
		1024,
		"the maximum number of logs to be processed in the queue (per source)",
	)
	flushInterval := fs.Duration("log-flush-interval", 1*time.Second, "the interval at which logs are flushed to disk")
	maxLogLinesBeforeFlush := fs.Uint32(
		"log-max-lines-before-flush",
		100,
		"maximum number of lines buffered before flushing to disk",
	)
	rotateInterval := fs.Duration("log-rotate-interval", 24*time.Hour, "the interval at which log files are rotated")
	maxFileSizeBeforeRotate := fs.Uint64(
		"log-max-file-size-before-rotate",
		10*1<<30,
		"maximum amount of bytes before a log is rotated",
	)

	httpHandlerFS, getHTTPHandlerConfig := httphandler.HTTPHandlerFlagSet()
	fs.AddFlagSet(httpHandlerFS)

	httpLogFS, getHTTPLogConfig := httplog.HTTPLogFlagSet()
	fs.AddFlagSet(httpLogFS)

	return fs, func() LogConfig {
		return LogConfig{
			Path:                    *path,
			Capacity:                *capacity,
			FlushInterval:           *flushInterval,
			MaxLogLinesBeforeFlush:  *maxLogLinesBeforeFlush,
			RotateInterval:          *rotateInterval,
			MaxFileSizeBeforeRotate: *maxFileSizeBeforeRotate,

			HTTPHandler: getHTTPHandlerConfig(),
			HTTPLog:     getHTTPLogConfig(),
		}
	}
}
