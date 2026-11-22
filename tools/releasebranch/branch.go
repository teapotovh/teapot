package main

import (
	"bytes"
	"encoding/xml"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/teapotovh/teapot/tools/util"

	_ "embed"
)

// ErrNoTag reports that no tag was given.
var ErrNoTag = errors.New("no tag given, pass -tag or set GITHUB_REF_NAME")

// ErrNoPush reports that no ghcr.io image_push target exists.
var ErrNoPush = errors.New("no ghcr.io image_push targets found")

// ErrOverlay reports that an overlay dir is already present.
var ErrOverlay = errors.New("overlay is not empty, aborting")

var (
	tag     = flag.String("tag", os.Getenv("GITHUB_REF_NAME"), "release tag")
	verbose = flag.Bool("verbose", false, "print all executed commands")
)

const (
	overlayDir     = "deploy/release"
	baseOverlayDir = "deploy/base/release"
	branchPrefix   = "release/"
	commitPrefix   = "release "
	registry       = "ghcr.io"
	botName        = "github-actions[bot]"
	botEmail       = "github-actions[bot]@users.noreply.github.com"
)

type queryAttr struct {
	Name  string `xml:"name,attr"`
	Value string `xml:"value,attr"`
}

type queryRule struct {
	Attrs []queryAttr `xml:"string"`
}

type queryResult struct {
	Rules []queryRule `xml:"rule"`
}

type overlayData struct {
	Tag    string
	Repo   string
	Images []string
}

//go:embed gitrepository.yaml
var gitRepositoryTemplate []byte

//go:embed images.patch.yaml
var imagesPatchTemplate []byte

//go:embed release.kustomization.yaml
var releaseKustomizationTemplate []byte

//go:embed base.kustomization.yaml
var baseKustomizationTemplate []byte

var overlayFiles = []struct {
	template []byte
	output   string
}{
	{gitRepositoryTemplate, filepath.Join(overlayDir, "gitrepository.yaml")},
	{imagesPatchTemplate, filepath.Join(overlayDir, "images-patch.yaml")},
	{releaseKustomizationTemplate, filepath.Join(overlayDir, "kustomization.yaml")},
	{baseKustomizationTemplate, filepath.Join(baseOverlayDir, "kustomization.yaml")},
}

func main() {
	flag.Parse()

	if err := branch(); err != nil {
		fmt.Fprintln(os.Stderr, "release-branch: "+err.Error())
		os.Exit(1)
	}
}

func branch() error {
	if *tag == "" {
		return ErrNoTag
	}

	util.SetVerbose(*verbose)

	root, err := util.WorkspaceRoot()
	if err != nil {
		return err
	}

	util.SetRoot(root)

	repo, err := util.Repository()
	if err != nil {
		return err
	}

	util.Phase("querying pushed images")

	images, err := pushedImages()
	if err != nil {
		return err
	}

	util.Phase("writing overlay to " + overlayDir)

	if err := writeOverlay(root, repo, images); err != nil {
		return err
	}

	util.Phase("committing branch " + branchPrefix + *tag)

	return commit()
}

func pushedImages() ([]string, error) {
	stdout, stderr, err := util.Run(nil, "bazel", "query", "--output=xml",
		"kind(\"image_push\", //...)")
	if err != nil {
		return nil, fmt.Errorf("bazel query failed: %w, %s", err, stderr.String())
	}

	var query queryResult

	if err := parseQuery(stdout.Bytes(), &query); err != nil {
		return nil, err
	}

	seen := make(map[string]bool)

	var images []string

	for _, rule := range query.Rules {
		image, ok := pushImage(rule)
		if ok && !seen[image] {
			seen[image] = true
			images = append(images, image)
		}
	}

	if len(images) == 0 {
		return nil, ErrNoPush
	}

	return images, nil
}

func parseQuery(data []byte, query *queryResult) error {
	// Bazel emits an xml 1.1 declaration, which Go's decoder rejects.
	if bytes.HasPrefix(data, []byte("<?xml")) {
		_, data, _ = bytes.Cut(data, []byte("\n"))
	}

	if err := xml.NewDecoder(bytes.NewReader(data)).Decode(query); err != nil {
		return fmt.Errorf("could not parse query output: %w", err)
	}

	return nil
}

func pushImage(rule queryRule) (string, bool) {
	var repo string

	for _, attr := range rule.Attrs {
		switch attr.Name {
		case "registry":
			if attr.Value != registry {
				return "", false
			}
		case "repository":
			repo = attr.Value
		}
	}

	if repo == "" {
		return "", false
	}

	return registry + "/" + repo, true
}

func writeOverlay(root, repo string, images []string) error {
	for _, dir := range []string{overlayDir, baseOverlayDir} {
		path := filepath.Join(root, dir)

		if _, err := os.Stat(path); err == nil {
			return fmt.Errorf("%s: %w", path, ErrOverlay)
		}
	}

	data := overlayData{Tag: *tag, Repo: repo, Images: images}

	for _, file := range overlayFiles {
		content, err := render(file.template, file.output, data)
		if err != nil {
			return err
		}

		path := filepath.Join(root, file.output)
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			return fmt.Errorf("could not create overlay dir: %w", err)
		}

		if err := os.WriteFile(path, content, 0o644); err != nil { //nolint:gosec
			return fmt.Errorf("could not write %s: %w", path, err)
		}
	}

	return nil
}

func render(content []byte, name string, data overlayData) ([]byte, error) {
	tmpl, err := template.New(name).Parse(string(content))
	if err != nil {
		return nil, fmt.Errorf("could not parse %s: %w", name, err)
	}

	var b bytes.Buffer

	if err := tmpl.Execute(&b, data); err != nil {
		return nil, fmt.Errorf("could not execute %s: %w", name, err)
	}

	return b.Bytes(), nil
}

func commit() error {
	branch := branchPrefix + *tag

	if _, err := util.GitRun(nil, "config", "user.name", botName); err != nil {
		return err
	}

	if _, err := util.GitRun(nil, "config", "user.email", botEmail); err != nil {
		return err
	}

	if _, err := util.GitRun(nil, "checkout", "-B", branch); err != nil {
		return err
	}

	args := make([]string, 0, len(overlayFiles)+1)
	args = append(args, "add")

	for _, file := range overlayFiles {
		args = append(args, file.output)
	}

	if _, err := util.GitRun(nil, args...); err != nil {
		return err
	}

	if _, err := util.GitRun(strings.NewReader(commitPrefix+*tag), "commit", "-F", "-"); err != nil {
		return err
	}

	if _, err := util.GitRun(nil, "push", "origin", branch); err != nil {
		return err
	}

	return nil
}
