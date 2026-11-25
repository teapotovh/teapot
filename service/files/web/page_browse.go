package web

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path"
	"strconv"
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

// here is the cleaned path of the current directory.
const here = "."

const parentDir = ".."

const browseEntriesCursorID = "cursor"

func (web *Web) Browse(w http.ResponseWriter, r *http.Request) (ui.Component, error) {
	auth := webauth.GetAuth(r)
	if auth == nil {
		return nil, webhandler.NewRedirectError(PathIndex, http.StatusFound)
	}

	dir, err := urlPath(PathBrowse, r.URL.Path)
	if err != nil {
		return nil, fmt.Errorf("could not get relative path: %w", err)
	}

	switch r.Method {
	case http.MethodGet:
		entries, next, err := web.listDir(r.Context(), auth.Username, dir, nil)
		if err != nil {
			return nil, err
		}

		var (
			segments = []entry{
				{
					name: auth.Username,
					path: "/",
					mode: os.ModeDir,
				},
			}
			segmentPath string
		)

		for segment := range strings.SplitSeq(dir, "/") {
			if segment == here {
				continue
			}

			segmentPath = path.Join(segmentPath, segment)
			segments = append(segments, entry{
				name: segment,
				path: segmentPath,
				mode: os.ModeDir,
			})
		}

		component := browse{
			path:     dir,
			user:     auth.Username,
			segments: segments,
			entries:  entries,
			cursor:   next,
		}

		return webhandler.NewPage(
			pagetitle.Title("Browse at "+dir, App),
			"Browse your files at "+dir,
			component,
		), nil

	case http.MethodPost:
		value := r.FormValue(browseEntriesCursorID)
		if value == "" {
			return nil, fmt.Errorf("missing cursor: %w", webhandler.ErrBadRequest)
		}

		cursor := store.Cursor(value)

		entries, next, err := web.listDir(r.Context(), auth.Username, dir, &cursor)
		if err != nil {
			return nil, err
		}

		return browseEntries{
			path:    dir,
			entries: entries,
			cursor:  next,
		}, nil

	default:
		return nil, fmt.Errorf("invalid method %q: %w", r.Method, webhandler.ErrBadRequest)
	}
}

// listDir returns one page of entries of a directory.
func (web *Web) listDir(
	ctx context.Context,
	username string,
	dir string,
	cursor *store.Cursor,
) ([]entry, *store.Cursor, error) {
	session, err := web.files.Session(ctx, username)
	if err != nil {
		return nil, nil, webhandler.NewInternalError(err, nil)
	}

	page, err := session.FS().List(ctx, storePath(dir), cursor)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, nil, webhandler.ErrNotFound
		}

		err = fmt.Errorf("could not read directory at %q: %w", dir, err)

		return nil, nil, webhandler.NewInternalError(err, nil)
	}

	return toEntries(dir, page.Inodes), page.Cursor, nil
}

func toEntries(dir string, inodes []store.Inode) []entry {
	var entries []entry

	for _, inode := range inodes {
		mode := os.FileMode(0)
		if inode.Kind == store.KindDirectory {
			mode = os.ModeDir
		}

		entries = append(entries, entry{
			name: inode.Name,
			path: path.Join(dir, inode.Name),
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
	padding: var(--size-3) var(--size-2) var(--size-3) calc(var(--size-2) * 2 + var(--size-3));

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
  grid-template-columns: var(--size-3) 20fr 5fr;
	gap: var(--size-2);
	align-items: center;

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
		grid-template-columns: var(--size-3) max-content 20fr 5fr;
		& .mode {
	    display: block;
		}
	}
`)

func entryHref(entry entry) string {
	href := PathDownloadAt(entry.path)
	if entry.mode == os.ModeDir {
		href = PathBrowseDirAt(entry.path)
	}

	return EscapePath(href)
}

func entryNode(ctx ui.Context, entry entry) g.Node {
	var checkbox = h.Div()
	if entry.name != parentDir {
		checkbox = components.Checkbox(
			ctx,
			"Select "+entry.name,
			h.Name(selectionPathID),
			h.Value(entry.name),
		)
	}

	var name g.Node
	if entry.mode == os.ModeDir {
		name = h.A(hx.Boost("true"), h.Href(entryHref(entry)), g.Text(entry.name))
	} else {
		name = h.A(h.Target("_blank"), h.Href(entryHref(entry)), g.Text(entry.name))
	}

	return g.Group{
		checkbox,
		h.Div(h.Class("mode"), g.Text(entry.mode.String())),
		h.Div(name),
		h.Div(h.Class("size"), g.Text(humanize.IBytes(entry.size))),
	}
}

func (b browse) Render(ctx ui.Context) g.Node {
	dir := storePath(b.path)

	entries := b.entries
	if dir != "/" {
		entries = append([]entry{
			{
				name: parentDir,
				path: path.Join(dir, parentDir),
				mode: os.ModeDir,
				size: 0,
			},
		}, entries...)
	}

	downloadURL := EscapePath(PathDownloadDirAt(b.path))

	return g.Group{
		h.Form(
			h.ID(browseSelectionFormID),
			h.Method(http.MethodPost),
			h.Action(downloadURL),
			hx.On("change", "updateBrowseSelection(this)"),

			h.Div(ctx.Class(browseTitleStyle),
				h.Div(
					g.Map(b.segments, func(segment entry) g.Node {
						href := entryHref(segment)

						return g.Group{
							h.A(hx.Boost("true"), h.Href(href), g.Text(segment.name)),
							h.Span(g.Text("/")),
						}
					}),
				),
				h.Div(h.Class("buttons"),
					components.Button(ctx,
						h.Type("submit"),
						h.FormAction(downloadURL),
						h.Disabled(),
						h.Hidden("true"),
						g.Attr("data-selection-action", selectionActionDownload),
						g.Text("Download"),
					),
					components.Button(ctx,
						h.Type("button"),
						hx.Post(PathBrowseDialogOf(BrowseDialogRename)),
						hx.Target("#"+components.DialogID+" #"+components.DialogContentID),
						hx.Swap("inner"),
						h.Disabled(),
						h.Hidden("true"),
						g.Attr("data-selection-action", selectionActionRename),
						g.Text("Rename"),
					),
					components.Button(ctx,
						h.Type("button"),
						hx.Post(PathBrowseDialogOf(BrowseDialogDelete)),
						hx.Target("#"+components.DialogID+" #"+components.DialogContentID),
						hx.Swap("inner"),
						h.Disabled(),
						h.Hidden("true"),
						g.Attr("data-selection-action", selectionActionDelete),
						g.Text("Delete"),
					),
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
		),

		h.Script(g.Raw(`
			function updateBrowseSelection(form) {
				var selected = form.querySelectorAll('input[name="` + selectionPathID + `"]:checked').length
				form.querySelectorAll('[data-selection-action]').forEach(function (button) {
					button.hidden = selected === 0
					if (button.dataset.selectionAction === '` + selectionActionRename + `') {
						button.disabled = selected !== 1
					} else {
						button.disabled = selected === 0
					}
				})
			}
		`)),
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
		nodes = append(nodes, h.Div(h.Class("more"),
			hx.Post(EscapePath(PathBrowseDirAt(be.path))),
			hx.Trigger("revealed"),
			hx.Swap("outerHTML"),
			hx.Vals(`{"`+browseEntriesCursorID+`":`+strconv.Quote(string(*be.cursor))+`}`),
			g.Text("Loading..."),
		))
	}

	return g.Group(nodes)
}

// Ensure browseEntries implements ui.Component.
var _ ui.Component = browseEntries{}
