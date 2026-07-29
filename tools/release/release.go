package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/bazelbuild/rules_go/go/runfiles"
)

var (
	remoteName = flag.String("remote", "origin", "git remote to push the tag to")
	dryRun     = flag.Bool("dry-run", false, "run all checks and print the planned tag, but don't tag or push")
	verbose    = flag.Bool("verbose", false, "print all executed commands")
)

var root string

// tagPattern matches tags produced by this tool: v0.0.0-<14 digit UTC
// timestamp>-<12 hex char short sha>. Kept intentionally narrow so that
// unrelated tags in the repo are never picked up as "the last release".
var tagPattern = regexp.MustCompile(`^v0\.0\.0-\d{14}-[0-9a-f]{12}$`)

var (
	ErrFirstRelease = errors.New("first release")
)

func main() {
	flag.Parse()

	if err := release(); err != nil {
		fmt.Fprintln(os.Stderr, "release: "+err.Error())
		os.Exit(1)
	}
}

var phaseN uint32

func phase(name string) {
	phaseN += 1
	fmt.Fprintf(os.Stderr, "%02d %s\n", phaseN, name)
}

func release() (err error) {
	buildifierPath, err := runfiles.Rlocation("multitool/tools/buildifier/workspace_root")
	if err != nil {
		return fmt.Errorf("could not fetch buildifier path: %w", err)
	}

	fmt.Println(buildifierPath)

	root, err = workspaceRoot()
	if err != nil {
		return err
	}

	phase("(check) gazelle")

	if err := checkGazelle(); err != nil {
		return fmt.Errorf("gazelle check failed: %w", err)
	}

	phase("(check) git clean")

	if err := checkClean(); err != nil {
		return err
	}

	phase("(check) golangci-lint")

	if err := runLint(); err != nil {
		return fmt.Errorf("lint failed: %w", err)
	}

	headSHA, err := gitRun(nil, "rev-parse", "HEAD")
	if err != nil {
		return fmt.Errorf("resolving HEAD: %w", err)
	}

	headShort, err := gitRun(nil, "rev-parse", "--short=12", "HEAD")
	if err != nil {
		return fmt.Errorf("resolving short HEAD: %w", err)
	}

	lastTag, err := lastRelease()
	if err != nil {
		return fmt.Errorf("finding last release tag: %w", err)
	}

	subjects, err := commitsSince(lastTag)
	if err != nil {
		return fmt.Errorf("listing commits since %s: %w", displayTag(lastTag), err)
	}

	if len(subjects) == 0 {
		return fmt.Errorf("no new commits since %s; nothing to release", displayTag(lastTag))
	}

	newTag := fmt.Sprintf("v0.0.0-%s-%s", time.Now().UTC().Format("20060102150405"), headShort)
	message := changelog(newTag, lastTag, subjects)

	fmt.Println("==> Release plan")
	fmt.Printf("    previous tag: %s\n", displayTag(lastTag))
	fmt.Printf("    new tag:      %s\n", newTag)
	fmt.Printf("    head:         %s\n", headSHA)
	fmt.Printf("    commits:      %d\n", len(subjects))
	fmt.Println()
	fmt.Println(message)

	if *dryRun {
		fmt.Println("==> --dry-run set, not tagging or pushing")
		return nil
	}

	fmt.Println("==> Creating annotated tag")

	if err := createTag(newTag, message); err != nil {
		return fmt.Errorf("creating tag: %w", err)
	}

	fmt.Printf("==> Pushing %s to %s\n", newTag, *remoteName)

	if err := pushTag(*remoteName, newTag); err != nil {
		return fmt.Errorf("pushing tag: %w", err)
	}

	fmt.Printf("Released %s\n", newTag)

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

// run executes git with args and returns trimmed stdout. On failure the
// error includes stderr, since git's error messages are usually the most
// useful part.
func gitRun(in io.Reader, args ...string) (string, error) {
	stdout, stderr, err := run(in, "git", args...)
	if err != nil {
		return "", fmt.Errorf("git %s: %w\n%s", strings.Join(args, " "), err, stderr.String())
	}

	return strings.TrimSpace(stdout.String()), nil
}

// checkGazelle shells out to bazel to verify generated BUILD files match
// what gazelle would produce, without writing anything. `-mode=diff`
// prints a diff and exits non-zero if the tree is stale.
func checkGazelle() error {
	_, _, err := run(nil, "bazel", "run", "//:gazelle", "--", "-mode=diff")
	return err
}

func runLint() error {
	_, _, err := run(nil, "golangci-lint", "run", "./...")
	return err
}

func checkClean() error {
	out, err := gitRun(nil, "status", "--porcelain")
	if err != nil {
		return fmt.Errorf("checking status: %w", err)
	}

	if out != "" {
		return fmt.Errorf("working tree is not clean:\n%s", out)
	}

	return nil
}

func run(stding io.Reader, command string, args ...string) (*bytes.Buffer, *bytes.Buffer, error) {
	cmd := exec.Command(command, args...)
	cmd.Dir = root

	var stdout, stderr bytes.Buffer

	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if *verbose {
		fmt.Printf("+ %s %s\n", command, strings.Join(args, " "))
	}

	return &stdout, &stderr, cmd.Run()
}

// lastRelease finds the most recent tag matching tagPattern. Because the
// pattern is a zero-padded, fixed-width timestamp, lexicographic sort order
// matches chronological order, so no extra commit-date lookups are needed.
// Returns "" if this is the first release.
func lastRelease() (string, error) {
	out, err := gitRun(nil, "tag", "--list")
	if err != nil {
		return "", err
	}

	var names []string

	for line := range strings.SplitSeq(out, "\n") {
		line = strings.TrimSpace(line)
		if tagPattern.MatchString(line) {
			names = append(names, line)
		}
	}

	if len(names) == 0 {
		return "", ErrFirstRelease
	}

	sort.Strings(names)

	return names[len(names)-1], nil
}

// commitsSince returns the subject line of every commit reachable from HEAD
// but not from lastTag (all of history, if lastTag is empty), newest first.
func commitsSince(lastTag string) ([]string, error) {
	rangeArg := "HEAD"
	if lastTag != "" {
		rangeArg = lastTag + "..HEAD"
	}

	out, err := gitRun(nil, "log", "--pretty=format:%s", rangeArg)
	if err != nil {
		return nil, err
	}

	if out == "" {
		return nil, nil
	}

	return strings.Split(out, "\n"), nil
}

func displayTag(tag string) string {
	if tag == "" {
		return "(none — first release)"
	}

	return tag
}

func changelog(newTag, lastTag string, subjects []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n\n", newTag)
	fmt.Fprintf(&b, "Changes since %s:\n\n", lastTag)

	for _, s := range subjects {
		fmt.Fprintf(&b, "* %s\n", s)
	}

	return b.String()
}

func createTag(name, message string) error {
	_, err := gitRun(strings.NewReader(message), "tag", "-a", name, "-F", "-")
	return err
}

func pushTag(remote, tag string) error {
	_, err := gitRun(nil, "push", remote, tag)
	return err
}
