package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/bazelbuild/rules_go/go/runfiles"

	"github.com/teapotovh/teapot/tools/util"
)

var (
	ErrNothingToRelease    = errors.New("nothing to release")
	ErrWorkingTreeNotClean = errors.New("working tree is not clean")
	ErrFirstRelease        = errors.New("first release")
	ErrDryRun              = errors.New("-dry-run set, aborting")
)

var (
	dryRun  = flag.Bool("dry-run", false, "run all checks and print the planned tag, but don't tag")
	dirty   = flag.Bool("dirty", false, "skips the clean check, but does not allow tagging")
	verbose = flag.Bool("verbose", false, "print all executed commands")
)

var (
	directories = []string{
		"cmd", "lib", "service",
		"proto", "tools",
	}
)

// tagPattern matches tags produced by this tool: v0.0.0-<14 digit UTC
// timestamp>-<12 hex char short sha>. Kept intentionally narrow so that
// unrelated tags in the repo are never picked up as "the last release".
var tagPattern = regexp.MustCompile(`^v0\.0\.0-\d{14}-[0-9a-f]{12}$`)

func main() {
	flag.Parse()

	if *dirty {
		// To prevent from tagging releases with dirty worktrees
		fmt.Fprintln(os.Stderr, "! -dirty automatically enables -dry-run")

		*dryRun = true
	}

	if err := release(); err != nil {
		fmt.Fprintln(os.Stderr, "release: "+err.Error())
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

	golangciLintPath, err := runfiles.Rlocation("multitool/tools/golangci-lint/golangci-lint")
	if err != nil {
		return fmt.Errorf("could not fetch golangci-lint path: %w", err)
	}

	util.SetVerbose(*verbose)

	root, err := util.WorkspaceRoot()
	if err != nil {
		return err
	}

	util.SetRoot(root)

	headShort, err := util.GitRun(nil, "rev-parse", "--short=12", "HEAD")
	if err != nil {
		return fmt.Errorf("resolving short HEAD: %w", err)
	}

	newTag := fmt.Sprintf("v0.0.0-%s-%s", time.Now().UTC().Format("20060102150405"), headShort)
	phase("(info) minting release: " + newTag)

	phase("(check) bazel mod tidy")

	if stdout, stderr, err := util.Run(nil, "bazel", "mod", "tidy"); err != nil {
		return fmt.Errorf("bazel mod tidy failed: %w, %s, %s", err, stdout.String(), stderr.String())
	}

	phase("(check) gazelle")

	if _, _, err := util.Run(nil, gazellePath, "-mode=diff"); err != nil {
		return fmt.Errorf("gazelle check failed: %w", err)
	}

	phase("(check) buildifier")

	for _, dir := range directories {
		if stdout, stderr, err := util.Run(nil, buildifierPath, "-mode=diff", "-r", dir); err != nil {
			return fmt.Errorf("buildifier failed: %w, %s, %s", err, stdout.String(), stderr.String())
		}
	}

	phase("(check) golangci-lint")

	if stdout, _, err := util.Run(nil, golangciLintPath, "run"); err != nil {
		fmt.Fprintf(os.Stderr, "%s", stdout.String())
		return fmt.Errorf("golangci-lint failed: %w", err)
	}

	phase("(check) clean working tree")

	out, err := util.GitRun(nil, "status", "--porcelain")
	if err != nil {
		return fmt.Errorf("checking status: %w", err)
	}

	if out != "" && !*dirty {
		fmt.Fprintf(os.Stderr, "%s", out)
		return ErrWorkingTreeNotClean
	}

	phase("(release) planning")

	headSHA, err := util.GitRun(nil, "rev-parse", "HEAD")
	if err != nil {
		return fmt.Errorf("resolving HEAD: %w", err)
	}

	lastTag, err := lastRelease()
	if err != nil {
		return fmt.Errorf("finding last release tag: %w", err)
	}

	subjects, err := commitsSince(lastTag)
	if err != nil {
		return fmt.Errorf("listing commits since %s: %w", lastTag, err)
	}

	if len(subjects) == 0 {
		return fmt.Errorf("no new commits since %s: %w", lastTag, ErrNothingToRelease)
	}

	message := changelog(newTag, lastTag, subjects)

	fmt.Println()
	fmt.Printf("\tprevious tag: %s\n", lastTag)
	fmt.Printf("\tnew tag:      %s\n", newTag)
	fmt.Printf("\thead:         %s\n", headSHA)
	fmt.Printf("\tcommits:      %d\n", len(subjects))
	fmt.Println()

	phase("(release) overview")
	fmt.Println()

	for line := range strings.SplitSeq(message, "\n") {
		fmt.Fprintln(os.Stderr, "\t"+line)
	}

	phase("(release) tagging")

	if *dryRun {
		return ErrDryRun
	}

	if _, err := util.GitRun(strings.NewReader(message), "tag", "-a", newTag, "-F", "-"); err != nil {
		return fmt.Errorf("creating tag: %w", err)
	}

	phase("(release) tagged, please run")

	_, err = fmt.Fprintln(os.Stderr, "git push origin "+newTag)

	return err
}

// lastRelease finds the most recent tag matching tagPattern. Because the
// pattern is a zero-padded, fixed-width timestamp, lexicographic sort order
// matches chronological order, so no extra commit-date lookups are needed.
// Returns "" if this is the first release.
func lastRelease() (string, error) {
	out, err := util.GitRun(nil, "tag", "--list")
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

	out, err := util.GitRun(nil, "log", "--pretty=format:%s", rangeArg)
	if err != nil {
		return nil, err
	}

	if out == "" {
		return nil, nil
	}

	return strings.Split(out, "\n"), nil
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
