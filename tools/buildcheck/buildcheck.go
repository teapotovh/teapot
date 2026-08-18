package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/bazelbuild/rules_go/go/runfiles"
)

var (
	ErrNothingToRelease    = errors.New("nothing to release")
	ErrWorkingTreeNotClean = errors.New("working tree is not clean")
	ErrFirstRelease        = errors.New("first release")
	ErrDryRun              = errors.New("-dry-run set, aborting")
)

var (
	verbose = flag.Bool("verbose", false, "print all executed commands")
)

var (
	root        string
	directories = []string{
		"cmd", "lib", "service",
		"proto", "tools",
	}
)

func main() {
	flag.Parse()

	if err := release(); err != nil {
		fmt.Fprintln(os.Stderr, "buildcheck: "+err.Error())
		os.Exit(1)
	}
}

var phaseN uint32

func phase(name string) {
	fmt.Fprintf(os.Stderr, "%02d %s\n", phaseN, name)
	phaseN += 1
}

//nolint:gocyclo
func release() (err error) {
	gazellePath, err := runfiles.Rlocation("com_github_teapotovh_teapot/tools/gazelle")
	if err != nil {
		return fmt.Errorf("could not fetch gazelle path: %w", err)
	}

	buildifierPath, err := runfiles.Rlocation("multitool/tools/buildifier/buildifier")
	if err != nil {
		return fmt.Errorf("could not fetch buildifier path: %w", err)
	}

	root, err = workspaceRoot()
	if err != nil {
		return err
	}

	phase("(check) bazel mod tidy")

	if stdout, stderr, err := run("bazel", "mod", "tidy"); err != nil {
		return fmt.Errorf("bazel mod tidy failed: %w, %s, %s", err, stdout.String(), stderr.String())
	}

	phase("(check) gazelle")

	if _, _, err := run(gazellePath, "-mode=diff"); err != nil {
		return fmt.Errorf("gazelle check failed: %w", err)
	}

	phase("(check) buildifier")

	for _, dir := range directories {
		if stdout, stderr, err := run(buildifierPath, "-mode=diff", "-r", dir); err != nil {
			return fmt.Errorf("buildifier failed: %w, %s, %s", err, stdout.String(), stderr.String())
		}
	}

	return nil
}

// workspaceRoot returns the real checkout directory. When launched via
// `bazel run`, Bazel sets BUILD_WORKSPACE_DIRECTORY to it; the binary
// itself starts out in a runfiles directory, not the checkout.
func workspaceRoot() (string, error) {
	if dir := os.Getenv("BUILD_WORKSPACE_DIRECTORY"); dir != "" {
		return dir, nil
	}

	return os.Getwd()
}

func run(command string, args ...string) (*bytes.Buffer, *bytes.Buffer, error) {
	cmd := exec.Command(command, args...) //nolint:gosec
	cmd.Dir = root

	var stdout, stderr bytes.Buffer

	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if *verbose {
		fmt.Printf("+ %s %s\n", command, strings.Join(args, " "))
	}

	return &stdout, &stderr, cmd.Run()
}
