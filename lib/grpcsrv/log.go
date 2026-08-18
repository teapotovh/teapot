package grpcsrv

import (
	"context"
	"fmt"
	"log/slog"
	"os"
)

type grpcLogger struct {
	logger *slog.Logger
}

//nolint:sloglint
func (s *grpcLogger) Info(args ...any) { s.logger.Info(fmt.Sprint(args...)) }

//nolint:sloglint
func (s *grpcLogger) Infoln(args ...any) { s.logger.Info(fmt.Sprintln(args...)) }

//nolint:sloglint
func (s *grpcLogger) Infof(format string, args ...any) { s.logger.Info(fmt.Sprintf(format, args...)) }

//nolint:sloglint
func (s *grpcLogger) Warning(args ...any) { s.logger.Warn(fmt.Sprint(args...)) }

//nolint:sloglint
func (s *grpcLogger) Warningln(args ...any) { s.logger.Warn(fmt.Sprintln(args...)) }

//nolint:sloglint
func (s *grpcLogger) Warningf(format string, args ...any) {
	s.logger.Warn(fmt.Sprintf(format, args...))
}

//nolint:sloglint
func (s *grpcLogger) Error(args ...any) { s.logger.Error(fmt.Sprint(args...)) }

//nolint:sloglint
func (s *grpcLogger) Errorln(args ...any) { s.logger.Error(fmt.Sprintln(args...)) }

//nolint:sloglint
func (s *grpcLogger) Errorf(format string, args ...any) { s.logger.Error(fmt.Sprintf(format, args...)) }

//nolint:sloglint
func (s *grpcLogger) Fatal(args ...any) { s.logger.Error(fmt.Sprint(args...)); os.Exit(1) }

//nolint:sloglint
func (s *grpcLogger) Fatalln(args ...any) { s.logger.Error(fmt.Sprintln(args...)); os.Exit(1) }

//nolint:sloglint
func (s *grpcLogger) Fatalf(format string, args ...any) {
	s.logger.Error(fmt.Sprintf(format, args...))
	os.Exit(1)
}

func (s *grpcLogger) V(l int) bool {
	if l >= 2 {
		return s.logger.Enabled(context.Background(), slog.LevelDebug)
	}

	return true
}
