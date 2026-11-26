package web

import (
	"bytes"
	"strings"
	"testing"

	g "maragu.dev/gomponents"

	"github.com/teapotovh/teapot/lib/ui"
)

type testContext struct{}

// Class implements ui.Context.
func (testContext) Class(_ ...*ui.Style) g.Node {
	return nil
}

func TestUploadDialogRender(t *testing.T) {
	t.Parallel()

	dialog := uploadDialog{
		url:  PathBrowseDialogOf(BrowseDialogUpload),
		path: "dir",
	}

	var buf bytes.Buffer

	if err := dialog.Render(testContext{}).Render(&buf); err != nil {
		t.Fatalf("Render: %v", err)
	}

	html := buf.String()

	if !strings.Contains(html, `id="`+uploadDialogErrorContainerID+`"`) {
		t.Errorf("upload dialog does not contain its error container")
	}

	if !strings.Contains(html, "multiple") {
		t.Errorf("upload dialog does not accept multiple files")
	}
}

func TestNewFolderDialogText(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	node := newFolderDialog{url: "/browse/newfolder", path: "/"}.Render(testContext{})
	if err := node.Render(&buf); err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	got := buf.String()

	for _, want := range []string{
		"Create a new folder in the current directory.",
		"Adding folders recursively is also supported.",
		"mkdir -p",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("dialog does not contain %q", want)
		}
	}
}
