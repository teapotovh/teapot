package main

import (
	"bytes"
	"strings"
	"testing"
)

// templateData mirrors the first release of this tool: two images, one
// per real service.
var templateData = overlayData{
	Tag:  "v0.0.0-20261122120000-abcdef123456",
	Repo: "github.com/teapotovh/teapot",
	Images: []string{
		"ghcr.io/teapotovh/kontakted",
		"ghcr.io/teapotovh/netd",
	},
}

func TestRenderTemplates(t *testing.T) {
	t.Parallel()

	for _, file := range overlayFiles {
		out, err := render(file.template, file.output, templateData)
		if err != nil {
			t.Fatal(err)
		}

		name := file.output

		// No template may carry unevaluated markers or empty lines.
		if strings.Contains(string(out), "{{") {
			t.Errorf("%s: unevaluated template:\n%s", name, out)
		}

		if strings.Contains(string(out), "\n\n") {
			t.Errorf("%s: empty line:\n%s", name, out)
		}

		if strings.Contains(name, "gitrepository") {
			for _, want := range []string{
				"url: https://github.com/teapotovh/teapot",
				"tag: v0.0.0-20261122120000-abcdef123456",
			} {
				if !bytes.Contains(out, []byte(want)) {
					t.Errorf("%s: missing %q:\n%s", name, want, out)
				}
			}

			continue
		}

		// The release kustomization is static, so only the remaining
		// templates must list one entry per image, in order.
		if strings.Contains(name, "kustomization.yaml") &&
			!strings.Contains(name, "base") {
			continue
		}

		first := bytes.Index(out, []byte("ghcr.io/teapotovh/kontakted"))

		if first < 0 {
			t.Errorf("%s: missing kontakted:\n%s", name, out)
		}

		if !bytes.Contains(out[first:], []byte("ghcr.io/teapotovh/netd")) {
			t.Errorf("%s: missing netd after kontakted:\n%s", name, out)
		}
	}
}
