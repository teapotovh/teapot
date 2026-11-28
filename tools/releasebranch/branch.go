package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"sigs.k8s.io/kustomize/api/types"
	"sigs.k8s.io/yaml"

	"github.com/teapotovh/teapot/tools/util"

	_ "embed"
)

// ErrNoTag reports that no tag was given.
var ErrNoTag = errors.New("no tag given, pass -tag or set GITHUB_REF_NAME")

// ErrNoPush reports that no ghcr.io image_push target exists.
var ErrNoPush = errors.New("no ghcr.io image_push targets found")

var (
	tag     = flag.String("tag", os.Getenv("GITHUB_REF_NAME"), "release tag")
	verbose = flag.Bool("verbose", false, "print all executed commands")
)

const (
	gitRepositoryFile = "deploy/release.yaml"
	versionsFile      = "deploy/versions/kustomization.yaml"
	branchPrefix      = "release/"
	commitPrefix      = "release "
	registry          = "ghcr.io"
	gitRepositoryName = "teapot"
	gitRepositoryNS   = "flux"
	botName           = "github-actions[bot]"
	botEmail          = "github-actions[bot]@users.noreply.github.com"
)

//go:embed gitrepository.yaml
var gitRepositoryTemplate []byte

type releaseData struct {
	Repo   string
	Branch string
}

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

	util.Phase("writing versions for " + *tag)

	if err := writeVersions(root, images); err != nil {
		return err
	}

	if err := writeGitRepository(root, repo); err != nil {
		return err
	}

	util.Phase("committing branch " + branchPrefix + *tag)

	return commit()
}

func pushedImages() ([]string, error) {
	query, err := util.Query[queryResult]("kind(\"image_push\", //...)")
	if err != nil {
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

// writeVersions writes the version component that kustomize uses to set the
// tag of every image pushed by this release.
func writeVersions(root string, images []string) error {
	component := types.Kustomization{
		APIVersion: types.ComponentVersion,
		Kind:       types.ComponentKind,
	}

	for _, image := range images {
		component.Images = append(component.Images, types.Image{Name: image, NewTag: *tag})
	}

	data, err := yaml.Marshal(component)
	if err != nil {
		return fmt.Errorf("could not marshal versions: %w", err)
	}

	return write(root, versionsFile, data)
}

func writeGitRepository(root, repo string) error {
	data, err := render(gitRepositoryTemplate, gitRepositoryFile, releaseData{
		Repo:   repo,
		Branch: branchPrefix + *tag,
	})
	if err != nil {
		return err
	}

	return write(root, gitRepositoryFile, data)
}

func render(content []byte, name string, data releaseData) ([]byte, error) {
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

func write(root, name string, data []byte) error {
	path := filepath.Join(root, name)

	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("could not create %s: %w", filepath.Dir(path), err)
	}

	if err := os.WriteFile(path, data, 0o644); err != nil { //nolint:gosec
		return fmt.Errorf("could not write %s: %w", path, err)
	}

	return nil
}

// commit creates the release branch at HEAD, stages every tracked change and
// pushes it. CI checkout leaves the push credentials in place.
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

	if _, err := util.GitRun(nil, "add", "-u"); err != nil {
		return err
	}

	if _, err := util.GitRun(nil, "add", gitRepositoryFile); err != nil {
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
