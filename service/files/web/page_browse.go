package web

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

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

var (
	sep  = "/"
	here = "."
)

func (web *Web) Browse(w http.ResponseWriter, r *http.Request) (ui.Component, error) {
	auth := webauth.GetAuth(r)
	if auth == nil {
		return nil, httphandler.NewRedirectError(PathIndex, http.StatusFound)
	}

	path, err := filepath.Rel(PathBrowse, r.URL.Path)
	if err != nil {
		return nil, errors.Join(fmt.Errorf("could not get relative path: %w", err), webhandler.ErrBadRequest)
	}
	path = filepath.Clean(path)

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
		entryPath := filepath.Clean(filepath.Join(path, e.Name()))

		stat, err := hackpadfs.Stat(session.FS(), entryPath)
		if err != nil {
			err = fmt.Errorf("could not stat file at %q: %w", path, err)
			return nil, httphandler.NewInternalError(err, nil)
		}

		size := stat.Size()
		entries = append(entries, entry{
			name: e.Name(),
			path: entryPath,
			mode: e.Type(),
			size: uint64(size), //nolint:gosec
		})
	}

	var (
		segments []entry = []entry{
			{
				name: auth.Username,
				path: sep,
				mode: os.ModeDir,
			},
		}
		segmentPath string
	)
	for segment := range strings.SplitSeq(path, string(filepath.Separator)) {
		if segment == here {
			continue
		}

		segmentPath = filepath.Join(segmentPath, segment)
		segments = append(segments, entry{
			name: segment,
			path: segmentPath,
			mode: os.ModeDir,
		})
	}

	component := browse{
		path:     path,
		user:     auth.Username,
		segments: segments,
		entries:  entries,
	}

	return webhandler.NewPage(
		pagetitle.Title("Browse at "+path, App),
		"Browse your files at "+path,
		component,
	), nil
}

type entry struct {
	name string
	path string
	mode os.FileMode
	size uint64
}

type browse struct {
	path     string
	user     string
	segments []entry
	entries  []entry
}

var BrowseTitleStyle = ui.MustParseStyle(`
	font-size: var(--font-size-2);
	padding: var(--size-3) var(--size-2);
`)

var BrowseStyle = ui.MustParseStyle(`
	display: grid;
	padding: 0 var(--size-2);
	width: 100%;
  grid-template-columns: 20fr 5fr;
	gap: var(--size-2);

	& .size {
	  text-align: right;
	}

	& .mode {
	  display: none;
	}
	@media (min-width: 768px) {
		grid-template-columns: 2fr 20fr 5fr;
		& .mode {
	    display: block;
		}
	}
`)

func (b browse) Render(ctx ui.Context) g.Node {
	path := filepath.Join(sep, b.path)

	entries := b.entries
	if path != sep {
		entries = append([]entry{
			{
				name: "..",
				path: filepath.Join(path, ".."),
				mode: os.ModeDir,
				size: 0,
			},
		}, entries...)
	}

	href := func(entry entry) string {
		var href string
		if entry.mode == os.ModeDir {
			href = PathBrowseAt(entry.path) + sep
		} else {
			href = PathFileAt(entry.path)
		}
		return href
	}

	return g.Group{
		h.Div(ctx.Class(BrowseTitleStyle),
			g.Map(b.segments, func(segment entry) g.Node {
				href := href(segment)
				return g.Group{
					h.A(hx.Boost("true"), h.Href(href), g.Text(segment.name)),
					h.Span(g.Text(sep)),
				}
			}),
		),
		h.Section(ctx.Class(BrowseStyle),
			g.Map(entries, func(entry entry) g.Node {
				href := href(entry)

				target := hx.Boost("true")
				if entry.mode != os.ModeDir {
					target = h.Target("_blank")
				}

				return g.Group{
					h.Div(h.Class("mode"), g.Text(entry.mode.String())),
					h.Div(h.A(target, h.Href(href), g.Text(entry.name))),
					h.Div(h.Class("size"), g.Text(humanize.IBytes(entry.size))),
				}
			}),
		),
	}
}

// Ensure browse implements ui.Component.
var _ ui.Component = browse{}
