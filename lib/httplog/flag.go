package httplog

import (
	flag "github.com/spf13/pflag"
)

func HTTPLogFlagSet() (*flag.FlagSet, func() HTTPLogConfig) {
	fs := flag.NewFlagSet("httplog", flag.ExitOnError)

	level := fs.String("httplog-level", "debug", "the log level for HTTP logs")
	ignore := fs.String(
		"httplog-ignore",
		"(static|favicon)",
		"a regular expression to catch paths that should not be logged",
	)

	return fs, func() HTTPLogConfig {
		return HTTPLogConfig{
			Level:  *level,
			Ignore: *ignore,
		}
	}
}
