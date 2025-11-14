package cmd

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"syscall"
)

var (
	ErrAlreadyStarted = errors.New("command has already been started")
	ErrNotStarted     = errors.New("process not running")
)

type Command struct {
	logger      *slog.Logger
	levelStdout slog.Level
	levelStderr slog.Level

	cmd *exec.Cmd
}

func NewCommand(logger *slog.Logger, levelStdout, levelStderr slog.Level) *Command {
	return &Command{
		logger:      logger,
		levelStdout: levelStdout,
		levelStderr: levelStderr,
	}
}

func (c *Command) Start(name string, args ...string) error {
	if c.cmd != nil {
		return ErrAlreadyStarted
	}

	c.logger.Debug("starting command", "name", "args", args)
	c.cmd = exec.Command(name, args...)
	stdout, err := c.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to get stdout pipe: %w", err)
	}

	stderr, err := c.cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to get stderr pipe: %w", err)
	}

	if err := c.cmd.Start(); err != nil {
		return fmt.Errorf("failed to start %q process: %w", name, err)
	}

	go c.logOutput(stdout, c.levelStdout)
	go c.logOutput(stderr, c.levelStderr)
	return nil
}

func (c *Command) logOutput(r io.Reader, level slog.Level) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		c.logger.Log(context.Background(), level, scanner.Text())
	}
}

func (c *Command) Signal(signal os.Signal) error {
	if c.cmd == nil || c.cmd.Process == nil {
		return ErrNotStarted
	}

	if err := c.cmd.Process.Signal(signal); err != nil {
		return fmt.Errorf("failed to send SIGHUP to process: %w", err)
	}

	return nil
}

func (c *Command) Stop() error {
	if err := c.Signal(syscall.SIGTERM); err != nil {
		return err
	}

	return c.cmd.Wait()
}

func (c *Command) Wait() error {
	if c.cmd == nil {
		return ErrNotStarted
	}
	return c.cmd.Wait()
}
