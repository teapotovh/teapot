package log

import (
	flag "github.com/spf13/pflag"
)

func LogFlagSet() (*flag.FlagSet, func() LogConfig) {
	fs := flag.NewFlagSet("log", flag.ExitOnError)

	return fs, func() LogConfig {
		return LogConfig{}
	}
}
