package main

import (
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/bazelbuild/rules_go/go/runfiles"

	"github.com/teapotovh/teapot/tools/util"
)

var (
	ErrNothingToRelease    = errors.New("nothing to release")
	ErrWorkingTreeNotClean = errors.New("working tree is not clean")
)

var (
	verbose = flag.Bool("verbose", false, "print all executed commands")
)

var (
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

func release() (err error) {
	gazellePath, err := runfiles.Rlocation("com_github_teapotovh_teapot/tools/gazelle")
	if err != nil {
		return fmt.Errorf("could not fetch gazelle path: %w", err)
	}

	buildifierPath, err := runfiles.Rlocation("multitool/tools/buildifier/buildifier")
	if err != nil {
		return fmt.Errorf("could not fetch buildifier path: %w", err)
	}

	util.SetVerbose(*verbose)

	root, err := util.WorkspaceRoot()
	if err != nil {
		return err
	}

	util.SetRoot(root)

	phase("(check) bazel mod tidy")

	if stdout, stderr, err := util.Run(nil, "bazel", "mod", "tidy"); err != nil {
		return fmt.Errorf("bazel mod tidy failed: %w, %s, %s", err, stdout.String(), stderr.String())
	}

	phase("(check) gazelle")

	if stdout, stderr, err := util.Run(nil, gazellePath, "-mode=diff"); err != nil {
		return fmt.Errorf("gazelle check failed: %w, %s, %s", err, stdout.String(), stderr.String())
	}

	phase("(check) buildifier")

	for _, dir := range directories {
		if stdout, stderr, err := util.Run(nil, buildifierPath, "-mode=diff", "-r", dir); err != nil {
			return fmt.Errorf("buildifier failed: %w, %s, %s", err, stdout.String(), stderr.String())
		}
	}

	phase("(check) clean working tree")

	out, err := util.GitRun(nil, "status", "--porcelain")
	if err != nil {
		return fmt.Errorf("checking status: %w", err)
	}

	if out != "" {
		fmt.Fprintf(os.Stderr, "%s", out)
		return ErrWorkingTreeNotClean
	}

	return nil
}
