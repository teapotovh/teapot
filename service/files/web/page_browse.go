package web

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/dustin/go-humanize"
	g "maragu.dev/gomponents"
	hx "maragu.dev/gomponents-htmx"
	h "maragu.dev/gomponents/html"

	"github.com/teapotovh/teapot/lib/pagetitle"
	"github.com/teapotovh/teapot/lib/ui"
	"github.com/teapotovh/teapot/lib/ui/components"
	"github.com/teapotovh/teapot/lib/webauth"
	"github.com/teapotovh/teapot/lib/webhandler"
	"github.com/teapotovh/teapot/service/files/store"
)

const (
	// sep is the separator used by URL and store paths.
	sep = "/"
	// here is the cleaned path of the current directory.
	here = "."
)

func (web *Web) Browse(w http.ResponseWriter, r *http.Request) (ui.Component, error) {
	switch r.Method {
	case http.MethodGet:
		auth := webauth.GetAuth(r)
		if auth == nil {
			return nil, webhandler.NewRedirectError(PathIndex, http.StatusFound)
		}

		path, err := filepath.Rel(PathBrowse, r.URL.Path)
		if err != nil {
			return nil, errors.Join(fmt.Errorf("could not get relative path: %w", err), webhandler.ErrBadRequest)
		}

		path = filepath.Clean(path)

		session, err := web.files.Session(r.Context(), auth.Username)
		if err != nil {
			return nil, webhandler.NewInternalError(err, nil)
		}

		page, err := session.FS().List(r.Context(), sep+path, nil)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return nil, webhandler.ErrNotFound
			}

			err = fmt.Errorf("could not read directory at %q: %w", path, err)

			return nil, webhandler.NewInternalError(err, nil)
		}

		entries := toEntries(path, page.Inodes)

		var (
			segments = []entry{
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
			cursor:   page.Cursor,
		}

		return webhandler.NewPage(
			pagetitle.Title("Browse at "+path, App),
			"Browse your files at "+path,
			component,
		), nil
	}

	return nil, fmt.Errorf("invalid method %q: %w", r.Method, webhandler.ErrBadRequest)
}

const browseEntriesCursorID = "cursor"

// BrowseEntries returns the next page of a folder as a fragment.
func (web *Web) BrowseEntries(w http.ResponseWriter, r *http.Request) (ui.Component, error) {
	if r.Method != http.MethodGet {
		return nil, fmt.Errorf("invalid method %q: %w", r.Method, webhandler.ErrBadRequest)
	}

	auth := webauth.GetAuth(r)
	if auth == nil {
		return nil, webhandler.NewRedirectError(PathIndex, http.StatusFound)
	}

	path, err := getDialogBasePath(r, PathBrowse)
	if err != nil {
		return nil, err
	}

	var cursor *store.Cursor

	if value := r.URL.Query().Get(browseEntriesCursorID); value != "" {
		c := store.Cursor(value)
		cursor = &c
	}

	session, err := web.files.Session(r.Context(), auth.Username)
	if err != nil {
		return nil, webhandler.NewInternalError(err, nil)
	}

	page, err := session.FS().List(r.Context(), sep+path, cursor)
	if err != nil {
		err = fmt.Errorf("could not read directory at %q: %w", path, err)

		return nil, webhandler.NewInternalError(err, nil)
	}

	return browseEntries{
		path:    path,
		entries: toEntries(path, page.Inodes),
		cursor:  page.Cursor,
	}, nil
}

func toEntries(path string, inodes []store.Inode) []entry {
	var entries []entry

	for _, inode := range inodes {
		mode := os.FileMode(0)
		if inode.Kind == store.KindDirectory {
			mode = os.ModeDir
		}

		entries = append(entries, entry{
			name: inode.Name,
			path: filepath.Clean(filepath.Join(path, inode.Name)),
			mode: mode,
			size: uint64(inode.Size), //nolint:gosec
		})
	}

	return entries
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
	cursor   *store.Cursor
}

var browseTitleStyle = ui.MustParseStyle(`
	font-size: var(--font-size-2);
	padding: var(--size-3) var(--size-2);

	display: flex;
	flex-direction: row;
	justify-content: space-between;
	align-items: center;

	& .buttons button {
	  margin-left: var(--size-4);
	}
`)

var browseStyle = ui.MustParseStyle(`
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

	& .more {
	  grid-column: 1 / -1;
	  text-align: center;
	  padding: var(--size-2) 0;
	}

	@media (min-width: 768px) {
		grid-template-columns: 2fr 20fr 5fr;
		& .mode {
	    display: block;
		}
	}
`)

func entryHref(entry entry) string {
	href := PathFileAt(entry.path)
	if entry.mode == os.ModeDir {
		href = PathBrowseAt(entry.path) + sep
	}

	return EscapePath(href)
}

func entryNode(ctx ui.Context, entry entry) g.Node {
	target := hx.Boost("true")
	if entry.mode != os.ModeDir {
		target = h.Target("_blank")
	}

	return g.Group{
		h.Div(h.Class("mode"), g.Text(entry.mode.String())),
		h.Div(h.A(target, h.Href(entryHref(entry)), g.Text(entry.name))),
		h.Div(h.Class("size"), g.Text(humanize.IBytes(entry.size))),
	}
}

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

	return g.Group{
		h.Div(ctx.Class(browseTitleStyle),
			h.Div(
				g.Map(b.segments, func(segment entry) g.Node {
					href := entryHref(segment)

					return g.Group{
						h.A(hx.Boost("true"), h.Href(href), g.Text(segment.name)),
						h.Span(g.Text(sep)),
					}
				}),
			),
			h.Div(h.Class("buttons"),
				components.DialogButton(ctx, PathBrowseDialogOf(BrowseDialogNewFolder), g.Text("New Folder")),
				components.DialogButton(ctx, PathBrowseDialogOf(BrowseDialogUpload), g.Text("Upload")),
			),
		),

		h.Section(ctx.Class(browseStyle),
			browseEntries{
				path:    b.path,
				entries: entries,
				cursor:  b.cursor,
			}.Render(ctx),
		),
	}
}

// Ensure browse implements ui.Component.
var _ ui.Component = browse{}

type browseEntries struct {
	path    string
	entries []entry
	cursor  *store.Cursor
}

func (be browseEntries) Render(ctx ui.Context) g.Node {
	nodes := make([]g.Node, 0, len(be.entries)+1)

	for _, entry := range be.entries {
		nodes = append(nodes, entryNode(ctx, entry))
	}

	if be.cursor != nil {
		query := url.Values{}
		query.Set(browseEntriesCursorID, string(*be.cursor))
		href := PathBrowseEntries + "?" + query.Encode()

		nodes = append(nodes, h.Div(h.Class("more"),
			hx.Get(href),
			hx.Trigger("revealed"),
			hx.Swap("outerHTML"),
			g.Text("Loading..."),
		))
	}

	return g.Group(nodes)
}

// Ensure browseEntries implements ui.Component.
var _ ui.Component = browseEntries{}
