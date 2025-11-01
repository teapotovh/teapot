package log

import (
	flag "github.com/spf13/pflag"
)

func LogFlagSet() (*flag.FlagSet, func() LogConfig) {
	fs := flag.NewFlagSet("log", flag.ExitOnError)

	level := fs.String("log-level", "info", "the log level for slog")
	format := fs.String("log-format", "text", "the log formatting, one of: text, tint, json")

	return fs, func() LogConfig {
		return LogConfig{
			Level:  *level,
			Format: *format,
		}
	}
}
