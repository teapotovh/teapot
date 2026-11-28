package main

import (
	"encoding/xml"
	"slices"
	"testing"
)

func TestQueryNames(t *testing.T) {
	t.Parallel()

	data := []byte(`<query version="2">
    <rule class="go_test" name="//lib/fs:fs_test">
        <string name="name" value="fs_test"/>
        <list name="srcs">
            <label value="//lib/fs:path_test.go"/>
        </list>
    </rule>
    <source-file name="//lib/fs:fs.go"/>
    <generated-file name="//proto/embed:embed.pb.go"/>
</query>`)

	var result queryNames
	if err := xml.Unmarshal(data, &result); err != nil {
		t.Fatalf("xml.Unmarshal() error = %v", err)
	}

	want := []string{"//lib/fs:fs_test", "//lib/fs:fs.go", "//proto/embed:embed.pb.go"}
	if !slices.Equal(result.names, want) {
		t.Errorf("names = %v, want %v", result.names, want)
	}
}

func TestRelativePath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		file string
		pkg  string
		want string
	}{
		{"lib/fs/fs.go", "lib/fs", "fs.go"},
		{"service/bottin/store/migrations/00001_x.sql", "service/bottin/store", "migrations/00001_x.sql"},
		{"deploy/app/kustomization.yaml", "", "deploy/app/kustomization.yaml"},
	}

	for _, test := range tests {
		if got := relativePath(test.file, test.pkg); got != test.want {
			t.Errorf("relativePath(%q, %q) = %q, want %q", test.file, test.pkg, got, test.want)
		}
	}
}

func TestIsGlobalChange(t *testing.T) {
	t.Parallel()

	tests := map[string]bool{
		"MODULE.bazel":        true,
		"go.mod":              true,
		".bazelrc":            true,
		"bazel/status.sh":     true,
		"tools/gazelle.bzl":   true,
		"lib/httpauth/jwt.go": false,
		"service/docs/a.md":   false,
	}

	for file, want := range tests {
		if got := isGlobalChange(file); got != want {
			t.Errorf("isGlobalChange(%q) = %v, want %v", file, got, want)
		}
	}
}

func TestIsGoChange(t *testing.T) {
	t.Parallel()

	tests := map[string]bool{
		"lib/httpauth/jwt.go":      true,
		"proto/files/files.proto":  true,
		"go.mod":                   true,
		".golangci.yaml":           true,
		"bazel/patches/tokenizers": true,
		"deploy/app/kustomization": false,
		"service/bottin/README.md": false,
	}

	for file, want := range tests {
		if got := isGoChange(file); got != want {
			t.Errorf("isGoChange(%q) = %v, want %v", file, got, want)
		}
	}
}

func TestIsManifestChange(t *testing.T) {
	t.Parallel()

	tests := map[string]bool{
		"deploy/base/kustomization.yaml":       true,
		"tools/manifestcheck/manifestcheck.go": true,
		"bazel/multitool.lock.json":            true,
		"go.mod":                               true,
		"lib/httpauth/jwt.go":                  false,
		"service/bottin/store/store.go":        false,
	}

	for file, want := range tests {
		if got := isManifestChange(file); got != want {
			t.Errorf("isManifestChange(%q) = %v, want %v", file, got, want)
		}
	}
}

func TestIsBuildChange(t *testing.T) {
	t.Parallel()

	tests := map[string]bool{
		"lib/httpauth/BUILD.bazel": true,
		"tools/buildcheck.go":      true,
		"tools/gazelle.bzl":        true,
		".bazelversion":            true,
		"deploy/app/kustomization": false,
		"README":                   false,
	}

	for file, want := range tests {
		if got := isBuildChange(file); got != want {
			t.Errorf("isBuildChange(%q) = %v, want %v", file, got, want)
		}
	}
}
