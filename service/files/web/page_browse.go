package web

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/dustin/go-humanize"
	"github.com/hack-pad/hackpadfs"
	g "maragu.dev/gomponents"
	hx "maragu.dev/gomponents-htmx"
	h "maragu.dev/gomponents/html"

	"github.com/teapotovh/teapot/lib/httphandler"
	"github.com/teapotovh/teapot/lib/pagetitle"
	"github.com/teapotovh/teapot/lib/ui"
	"github.com/teapotovh/teapot/lib/webauth"
	"github.com/teapotovh/teapot/lib/webhandler"
)

type entry struct {
	name string
	path string
	typ  os.FileMode
	size uint64
}

type browse struct {
	path    string
	entries []entry
}

func (b browse) Render(ctx ui.Context) g.Node {
	path := filepath.Join("/", b.path)

	return h.H1(
		g.Textf("browsing at %s", path),
		h.Ul(
			g.Map(b.entries, func(entry entry) g.Node {
				text := g.Textf("%s - %s - %s", entry.typ, entry.name, humanize.IBytes(entry.size))

				return h.Li(h.A(hx.Boost("true"), h.Href(PathBrowseAt(entry.path)), text))
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

	dirEntries, err := hackpadfs.ReadDir(session.FS(), path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, httphandler.ErrNotFound
		}

		err = fmt.Errorf("could not read directory at %q: %w", path, err)

		return nil, httphandler.NewInternalError(err, nil)
	}

	var entries []entry

	for _, e := range dirEntries {
		entryPath := filepath.Join(path, e.Name())

		stat, err := hackpadfs.Stat(session.FS(), entryPath)
		if err != nil {
			err = fmt.Errorf("could not stat file at %q: %w", path, err)
			return nil, httphandler.NewInternalError(err, nil)
		}

		size := stat.Size()
		entries = append(entries, entry{
			name: e.Name(),
			path: entryPath,
			typ:  e.Type(),
			size: uint64(size), //nolint:gosec
		})
	}

	component := browse{
		path:    path,
		entries: entries,
	}

	return webhandler.NewPage(
		pagetitle.Title("Browse at "+path, App),
		"Browse your files at "+path,
		component,
	), nil
}
