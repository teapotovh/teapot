package log

import (
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/lmittmann/tint"
)

var (
	ErrInvalidLevel  = errors.New("invalid level")
	ErrInvalidFormat = errors.New("invalid format")
)

type LogConfig struct {
	Level  string
	Format string
}

func NewLogger(config LogConfig) (*slog.Logger, error) {
	var level slog.Level
	if err := level.UnmarshalText([]byte(config.Level)); err != nil {
		return nil, fmt.Errorf("could not parse log level %q: %w", config.Level, ErrInvalidLevel)
	}

	var handler slog.Handler

	opts := &slog.HandlerOptions{
		Level: level,
	}

	switch config.Format {
	case "json":
		handler = slog.NewJSONHandler(os.Stdout, opts)
	case "text":
		handler = slog.NewTextHandler(os.Stdout, opts)
	case "tint":
		handler = tint.NewHandler(os.Stdout, &tint.Options{Level: level})
	default:
		return nil, fmt.Errorf("could not parse log format %q: %w", config.Format, ErrInvalidFormat)
	}

	return slog.New(handler), nil
}
