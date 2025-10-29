package log

import (
	flag "github.com/spf13/pflag"
)

func LogFlagSet() (*flag.FlagSet, func() LogConfig) {
	fs := flag.NewFlagSet("log", flag.ExitOnError)

	level := fs.String("log-level", "debug", "The log level for slog")
	format := fs.String("log-format", "text", "The log formatting, one of: text, tint, json")

	return fs, func() LogConfig {
		return LogConfig{
			Level:  *level,
			Format: *format,
		}
	}
}
