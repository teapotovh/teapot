package web

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path"
	"path/filepath"

	g "maragu.dev/gomponents"
	hx "maragu.dev/gomponents-htmx"
	h "maragu.dev/gomponents/html"

	"github.com/hack-pad/hackpadfs"
	"github.com/teapotovh/teapot/lib/httphandler"
	"github.com/teapotovh/teapot/lib/pagetitle"
	"github.com/teapotovh/teapot/lib/ui"
	"github.com/teapotovh/teapot/lib/webauth"
	"github.com/teapotovh/teapot/lib/webhandler"
)

type browse struct {
	path    string
	entries []hackpadfs.DirEntry
}

func (b browse) Render(ctx ui.Context) g.Node {
	path := path.Join("/", b.path)
	return h.H1(
		g.Textf("browsing at %s", path),
		h.Ul(
			g.Map(b.entries, func(entry hackpadfs.DirEntry) g.Node {
				text := g.Textf("%s - %s", entry.Type().String(), entry.Name())

				return h.Li(h.A(hx.Boost("true"), h.Href(PathBrowseAt(entry.Name())), text))
			}),
		),
	)
}

// Ensure browse implements ui.Component.
var _ ui.Component = browse{}

func (web *Web) Browse(w http.ResponseWriter, r *http.Request) (ui.Component, error) {
	auth := webauth.GetAuth(r)
	if auth == nil {
		return nil, httphandler.NewRedirectError(PathIndex, http.StatusFound)
	}

	path, err := filepath.Rel(PathBrowse, r.URL.Path)
	if err != nil {
		return nil, errors.Join(fmt.Errorf("could not get relative path: %w", err), webhandler.ErrBadRequest)
	}

	session, err := web.files.Sesssions().Get(auth.Username)
	if err != nil {
		return nil, httphandler.NewInternalError(err, nil)
	}

	entries, err := hackpadfs.ReadDir(session.FS(), path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, httphandler.ErrNotFound
		}

		return nil, fmt.Errorf("could not open file at %q: %w", path, err)
	}

	component := browse{
		path:    path,
		entries: entries,
	}
	return webhandler.NewPage(
		pagetitle.Title(fmt.Sprintf("Browse at %s", path), App),
		fmt.Sprintf("Browse your files at %s", path),
		component,
	), nil
}
