package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

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

const workflowsDir = ".github/workflows"

var bazelRE = regexp.MustCompile(`bazel (?:run|build|test) ([^\s"']+)`)

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

// checkWorkflowTargets verifies that every bazel target used in a workflow
// exists. A label typo in a workflow otherwise fails only in CI.
func checkWorkflowTargets(root string) error {
	entries, err := os.ReadDir(filepath.Join(root, workflowsDir))
	if err != nil {
		return fmt.Errorf("could not read %s: %w", workflowsDir, err)
	}

	seen := make(map[string]bool)

	var labels []string

	for _, entry := range entries {
		switch filepath.Ext(entry.Name()) {
		case ".yaml", ".yml":
		default:
			continue
		}

		data, err := os.ReadFile(filepath.Join(root, workflowsDir, entry.Name())) //nolint:gosec
		if err != nil {
			return fmt.Errorf("could not read workflow: %w", err)
		}

		for _, match := range bazelRE.FindAllStringSubmatch(string(data), -1) {
			label := match[1]

			if seen[label] || label == "//..." || strings.ContainsAny(label, "$'\"") {
				continue
			}

			seen[label] = true

			labels = append(labels, label)
		}
	}

	if len(labels) == 0 {
		return nil
	}

	expr := "set(" + strings.Join(labels, " ") + ")"

	if _, stderr, err := util.Run(nil, "bazel", "query", expr); err != nil {
		return fmt.Errorf("workflow target check failed: %w, %s", err, stderr.String())
	}

	return nil
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

	phase("(check) workflow targets")

	if err := checkWorkflowTargets(root); err != nil {
		return err
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
