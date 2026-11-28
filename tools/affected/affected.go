package main

import (
	"encoding/xml"
	"errors"
	"flag"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/teapotovh/teapot/tools/util"
)

// ErrUsage reports a missing or unknown subcommand.
var ErrUsage = errors.New("usage: affected <targets|matches> [flags]")

// ErrNoBase reports that no base revision was given.
var ErrNoBase = errors.New("no base revision, pass -base")

// ErrNoFilter reports that no file filter was given.
var ErrNoFilter = errors.New("no filter, pass -filter go, build, or manifests")

// ErrUnknownFilter reports an unknown file filter.
var ErrUnknownFilter = errors.New("unknown filter")

const (
	cmdMatches = "matches"
	cmdTargets = "targets"

	filterBuild     = "build"
	filterGo        = "go"
	filterManifests = "manifests"

	integrationTag = "integration"

	allTargets = "//..."
)

var workspace string

type queryNames struct {
	names []string
}

func (q *queryNames) UnmarshalXML(decoder *xml.Decoder, _ xml.StartElement) error {
	depth := 0

	for {
		token, err := decoder.Token()
		if err != nil {
			return fmt.Errorf("could not decode query output: %w", err)
		}

		switch element := token.(type) {
		case xml.StartElement:
			if depth == 0 {
				for _, attr := range element.Attr {
					if attr.Name.Local == "name" {
						q.names = append(q.names, attr.Value)

						break
					}
				}
			}

			depth++
		case xml.EndElement:
			if depth == 0 {
				return nil
			}

			depth--
		}
	}
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "affected: "+err.Error())
		os.Exit(1)
	}
}

//nolint:gocyclo // command dispatch keeps the shared logic in one place.
func run(args []string) error {
	if len(args) == 0 {
		return ErrUsage
	}

	command := args[0]
	if command != cmdTargets && command != cmdMatches {
		return fmt.Errorf("%w: %s", ErrUsage, command)
	}

	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	base := flags.String("base", "", "git revision to compare against")
	tag := flags.String("tag", "", "select only tests with this tag")
	filter := flags.String("filter", "", "file filter to match: go, build, or manifests")
	verbose := flags.Bool("verbose", false, "print all executed commands")

	if err := flags.Parse(args[1:]); err != nil {
		return err
	}

	if *base == "" {
		return ErrNoBase
	}

	util.SetVerbose(*verbose)

	root, err := util.WorkspaceRoot()
	if err != nil {
		return err
	}

	workspace = root

	util.SetRoot(root)

	changed, deleted, err := changedFiles(*base)
	if err != nil {
		return err
	}

	switch command {
	case cmdTargets:
		labels, everything, err := affectedLabels(changed, deleted)
		if err != nil {
			return err
		}

		if everything {
			fmt.Println(allTargets)

			return nil
		}

		if len(labels) == 0 {
			return nil
		}

		targets, err := testTargets(labels, *tag)
		if err != nil {
			return err
		}

		for _, target := range targets {
			fmt.Println(target)
		}

		return nil
	case cmdMatches:
		var matcher func(string) bool

		switch *filter {
		case "":
			return ErrNoFilter
		case filterGo:
			matcher = isGoChange
		case filterBuild:
			matcher = isBuildChange
		case filterManifests:
			matcher = isManifestChange
		default:
			return fmt.Errorf("%w: %s", ErrUnknownFilter, *filter)
		}

		if slices.ContainsFunc(changed, matcher) {
			fmt.Println(true)

			return nil
		}

		fmt.Println(false)

		return nil
	}

	return fmt.Errorf("%w: %s", ErrUsage, command)
}

func changedFiles(base string) ([]string, []string, error) {
	changed, err := gitDiff(base, "ACMRD")
	if err != nil {
		return nil, nil, err
	}

	deleted, err := gitDiff(base, "D")
	if err != nil {
		return nil, nil, err
	}

	return splitLines(changed), splitLines(deleted), nil
}

func gitDiff(base, filter string) (string, error) {
	out, err := util.GitRun(nil, "diff", "--name-only", "--diff-filter="+filter, base+"...HEAD")
	if err != nil {
		return "", err
	}

	return out, nil
}

func affectedLabels(changed, deleted []string) ([]string, bool, error) {
	removed := make(map[string]bool, len(deleted))
	for _, file := range deleted {
		removed[file] = true
	}

	cache := make(map[string][]string)
	labels := make(map[string]bool)

	for _, file := range changed {
		if isGlobalChange(file) {
			return nil, true, nil
		}

		pkg, ok := findPackage(path.Dir(file))
		if !ok {
			warnUndeclared(file)

			continue
		}

		files, ok, err := packageFiles(cache, pkg)
		if err != nil {
			return nil, false, err
		}

		if !ok {
			warnUndeclared(file)

			continue
		}

		if base := path.Base(file); base == "BUILD" || base == "BUILD.bazel" {
			for _, label := range files {
				labels[label] = true
			}

			continue
		}

		if removed[file] {
			for _, label := range files {
				labels[label] = true
			}

			continue
		}

		label := "//" + pkg + ":" + relativePath(file, pkg)
		if slices.Contains(files, label) {
			labels[label] = true
		} else {
			warnUndeclared(file)
		}
	}

	result := make([]string, 0, len(labels))
	for label := range labels {
		result = append(result, label)
	}

	slices.Sort(result)

	return result, false, nil
}

func relativePath(file, pkg string) string {
	if pkg == "" {
		return file
	}

	return strings.TrimPrefix(strings.TrimPrefix(file, pkg), "/")
}

func findPackage(dir string) (string, bool) {
	for {
		if hasBuildFile(dir) {
			return dir, true
		}

		if dir == "" {
			return "", false
		}

		dir = path.Dir(dir)
		if dir == "." {
			dir = ""
		}
	}
}

func packageFiles(cache map[string][]string, pkg string) ([]string, bool, error) {
	if cached, ok := cache[pkg]; ok {
		return cached, len(cached) > 0, nil
	}

	result, err := util.Query[queryNames]("//" + pkg + ":*")
	if err != nil {
		return nil, false, err
	}

	cache[pkg] = result.names

	return result.names, len(result.names) > 0, nil
}

func hasBuildFile(dir string) bool {
	for _, name := range []string{"BUILD.bazel", "BUILD"} {
		buildFile := filepath.Join(workspace, dir, name)
		if _, err := os.Stat(buildFile); err == nil { //nolint:gosec // the path comes from git.
			return true
		}
	}

	return false
}

func testTargets(labels []string, tag string) ([]string, error) {
	expression := "rdeps(//..., set(" + strings.Join(labels, " ") + "))"

	var query string
	if tag == "" {
		// The unit job filters integration tests at test time. Bazel fails
		// when every selected target is filtered out, so remove them here.
		query = fmt.Sprintf("kind(%q, %s) except attr(tags, %q, //...)", "go_test", expression, integrationTag)
	} else {
		query = fmt.Sprintf("kind(%q, attr(tags, %q, %s))", "go_test", tag, expression)
	}

	result, err := util.Query[queryNames](query)
	if err != nil {
		return nil, err
	}

	slices.Sort(result.names)

	return result.names, nil
}

func warnUndeclared(file string) {
	if isSource(file) || strings.HasSuffix(file, ".bzl") {
		fmt.Fprintf(os.Stderr, "affected: %s is not a Bazel input, skipping\n", file)
	}
}

func splitLines(s string) []string {
	var lines []string

	for line := range strings.SplitSeq(strings.TrimSpace(s), "\n") {
		if line != "" {
			lines = append(lines, line)
		}
	}

	return lines
}

func isSource(file string) bool {
	return strings.HasSuffix(file, ".go") || strings.HasSuffix(file, ".proto")
}

func isGlobalChange(file string) bool {
	if strings.HasPrefix(file, "bazel/") || strings.HasSuffix(file, ".bzl") {
		return true
	}

	switch file {
	case "MODULE.bazel", "MODULE.bazel.lock", "REPO.bazel", "WORKSPACE", "WORKSPACE.bazel",
		"go.mod", "go.sum", ".bazelrc", ".bazelversion":
		return true
	}

	return false
}

func isGoChange(file string) bool {
	if isSource(file) || strings.HasPrefix(file, "bazel/") {
		return true
	}

	switch file {
	case "go.mod", "go.sum", ".golangci.yaml", "MODULE.bazel", "MODULE.bazel.lock":
		return true
	}

	return false
}

func isBuildChange(file string) bool {
	if isGoChange(file) {
		return true
	}

	if strings.HasPrefix(file, "tools/") || strings.HasSuffix(file, ".bzl") {
		return true
	}

	switch file {
	case ".bazelrc", ".bazelversion", "REPO.bazel", "WORKSPACE", "WORKSPACE.bazel":
		return true
	}

	base := path.Base(file)

	return base == "BUILD" || base == "BUILD.bazel"
}

func isManifestChange(file string) bool {
	if strings.HasPrefix(file, "deploy/") ||
		strings.HasPrefix(file, "tools/manifestcheck/") ||
		strings.HasPrefix(file, "bazel/") {
		return true
	}

	switch file {
	case "MODULE.bazel", "MODULE.bazel.lock", "go.mod", "go.sum":
		return true
	}

	return false
}
