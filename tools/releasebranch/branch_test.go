package main

import (
	"os"
	"strings"
	"testing"

	"sigs.k8s.io/kustomize/api/types"
	"sigs.k8s.io/yaml"
)

func TestWriteVersions(t *testing.T) {
	t.Parallel()

	*tag = "v0.0.0-test"

	root := t.TempDir()

	images := []string{"ghcr.io/teapotovh/kontakted", "ghcr.io/teapotovh/netd"}

	if err := writeVersions(root, images); err != nil {
		t.Fatal(err)
	}

	var component types.Kustomization
	if err := yaml.Unmarshal([]byte(readFile(t, root+"/"+versionsFile)), &component); err != nil {
		t.Fatal(err)
	}

	if component.Kind != types.ComponentKind {
		t.Errorf("expected component, got %s", component.Kind)
	}

	if len(component.Images) != len(images) {
		t.Fatalf("expected %d images, got %d", len(images), len(component.Images))
	}

	for i, image := range component.Images {
		if image.Name != images[i] {
			t.Errorf("expected %s, got %s", images[i], image.Name)
		}

		if image.NewTag != "v0.0.0-test" {
			t.Errorf("expected the release tag, got %s", image.NewTag)
		}
	}
}

func TestWriteGitRepository(t *testing.T) {
	t.Parallel()

	*tag = "v0.0.0-test"

	root := t.TempDir()

	if err := writeGitRepository(root, "github.com/teapotovh/teapot"); err != nil {
		t.Fatal(err)
	}

	data := readFile(t, root+"/"+gitRepositoryFile)

	for _, want := range []string{
		"url: https://github.com/teapotovh/teapot",
		"branch: release/v0.0.0-test",
	} {
		if !strings.Contains(data, want) {
			t.Errorf("missing %q:\n%s", want, data)
		}
	}

	if strings.Contains(data, "{{") {
		t.Errorf("unevaluated template:\n%s", data)
	}
}

func readFile(t *testing.T, file string) string {
	t.Helper()

	data, err := os.ReadFile(file) //nolint:gosec
	if err != nil {
		t.Fatal(err)
	}

	return string(data)
}
