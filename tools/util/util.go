package util

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/bazelbuild/rules_go/go/runfiles"
)

var (
	root    string
	verbose bool
)

// WorkspaceRoot returns the real checkout directory. When launched via
// `bazel run`, Bazel sets BUILD_WORKSPACE_DIRECTORY to it; the binary
// itself starts out in a runfiles directory, not the checkout.
func WorkspaceRoot() (string, error) {
	if dir := os.Getenv("BUILD_WORKSPACE_DIRECTORY"); dir != "" {
		return dir, nil
	}

	return os.Getwd()
}

func SetRoot(r string) {
	root = r
}

func SetVerbose(v bool) {
	verbose = v
}

func Run(stdin io.Reader, command string, args ...string) (*bytes.Buffer, *bytes.Buffer, error) {
	cmd := exec.Command(command, args...) //nolint:gosec

	env, err := runfiles.Env()
	if err != nil {
		return nil, nil, fmt.Errorf("could not get runfiles environment: %w", err)
	}

	cmd.Env = append(cmd.Env, os.Environ()...)
	cmd.Env = append(cmd.Env, env...)
	cmd.Dir = root

	var stdout, stderr bytes.Buffer

	cmd.Stdin = stdin
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if verbose {
		fmt.Printf("+ %s %s\n", command, strings.Join(args, " "))
	}

	return &stdout, &stderr, cmd.Run()
}

// GitRun executes git with args and returns trimmed stdout. On failure the
// error includes stderr, since git's error messages are usually the most
// useful part.
func GitRun(in io.Reader, args ...string) (string, error) {
	stdout, stderr, err := Run(in, "git", args...)
	if err != nil {
		return "", fmt.Errorf("git %s: %w\n%s", strings.Join(args, " "), err, stderr.String())
	}

	return strings.TrimSpace(stdout.String()), nil
}
