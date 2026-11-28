package util

import (
	"bytes"
	"encoding/xml"
	"errors"
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
	phaseN  uint32
)

const repositoryRunfile = "com_github_teapotovh_teapot/bazel/repository.txt"

// ErrNoRepository reports that git remote origin is missing, so the
// normalized repository origin cannot be determined.
var ErrNoRepository = errors.New("could not determine repository origin, set git remote origin")

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

// RunfileString returns the trimmed content of a runfiles data file.
func RunfileString(rel string) (string, error) {
	path, err := runfiles.Rlocation(rel)
	if err != nil {
		return "", fmt.Errorf("could not locate runfile %s: %w", rel, err)
	}

	data, err := os.ReadFile(path) //nolint:gosec
	if err != nil {
		return "", fmt.Errorf("could not read runfile %s: %w", rel, err)
	}

	return strings.TrimSpace(string(data)), nil
}

// Repository returns the normalized origin, for example
// github.com/teapotovh/teapot, stamped by bazel/status.sh.
func Repository() (string, error) {
	repo, err := RunfileString(repositoryRunfile)
	if err != nil {
		return "", err
	}

	if repo == "" {
		return "", ErrNoRepository
	}

	return repo, nil
}

// Phase prints a numbered progress line to stderr.
func Phase(name string) {
	fmt.Fprintf(os.Stderr, "%02d %s\n", phaseN, name)
	phaseN += 1
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

// Query runs a bazel query and decodes the XML result into a T.
func Query[T any](expression string) (T, error) {
	var result T

	stdout, stderr, err := Run(nil, "bazel", "query", "--output=xml", expression)
	if err != nil {
		return result, fmt.Errorf("bazel query %q failed: %w, %s", expression, err, stderr.String())
	}

	data := stdout.Bytes()

	// Bazel emits an XML 1.1 declaration, which the Go decoder rejects.
	if bytes.HasPrefix(data, []byte("<?xml")) {
		_, data, _ = bytes.Cut(data, []byte("\n"))
	}

	decoder := xml.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&result); err != nil {
		return result, fmt.Errorf("could not decode query output: %w", err)
	}

	return result, nil
}
