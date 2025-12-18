package web

import (
	"errors"
	"fmt"
	"net/http"
	"path/filepath"

	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"

	"github.com/teapotovh/teapot/lib/pagetitle"
	"github.com/teapotovh/teapot/lib/ui"
	"github.com/teapotovh/teapot/lib/webhandler"
)

type browse struct {
	path string
}

func (b browse) Render(ctx ui.Context) g.Node {
	return h.H1(g.Textf("browsing at %s", b.path))
}

// Ensure browse implements ui.Component.
var _ ui.Component = browse{}

func (web *Web) Browse(w http.ResponseWriter, r *http.Request) (ui.Component, error) {
	path, err := filepath.Rel(PathBrowse, r.URL.Path)
	if err != nil {
		return nil, errors.Join(fmt.Errorf("could not get relative path: %w", err), webhandler.ErrBadRequest)
	}

	component := browse{
		path: path,
	}
	return webhandler.NewPage(
		pagetitle.Title(fmt.Sprintf("Browse at %s", path), App),
		fmt.Sprintf("Browse your files at %s", path),
		component,
	), nil
}
