package web

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"

	"github.com/hack-pad/hackpadfs"
	"github.com/kataras/muxie"
	g "maragu.dev/gomponents"
	hx "maragu.dev/gomponents-htmx"
	hxhttp "maragu.dev/gomponents-htmx/http"
	h "maragu.dev/gomponents/html"

	"github.com/teapotovh/teapot/lib/ui"
	"github.com/teapotovh/teapot/lib/ui/components"
	"github.com/teapotovh/teapot/lib/webauth"
	"github.com/teapotovh/teapot/lib/webhandler"
	"github.com/teapotovh/teapot/service/files"
)

const (
	BrowseDialogNewFolder = "newfolder"
	BrowseDialogUpload    = "upload"
)

var ErrInvalidBrowseDialog = errors.New("invalid browse dialog")

func (web *Web) BrowseDialog(w http.ResponseWriter, r *http.Request) (ui.Component, error) {
	dialog := muxie.GetParam(w, "dialog")
	switch dialog {
	case BrowseDialogNewFolder:
		return web.BrowseDialogNewFolder(w, r)

	case BrowseDialogUpload:
		return web.BrowseDialogUpload(w, r)

	default:
		return nil, fmt.Errorf("could not serve requested dialog %q: %w", dialog, ErrInvalidBrowseDialog)
	}
}

func (web *Web) BrowseDialogNewFolder(w http.ResponseWriter, r *http.Request) (ui.Component, error) {
	auth := webauth.GetAuth(r)
	if auth == nil {
		return nil, webhandler.NewRedirectError(PathIndex, http.StatusFound)
	}

	switch r.Method {
	case http.MethodGet:
		curr := hxhttp.GetCurrentURL(r.Header)
		url, err := url.Parse(curr)
		if err != nil {
			return nil, errors.Join(fmt.Errorf("could not parse current URL %q : %w", curr, err), webhandler.ErrBadRequest)
		}

		path, err := filepath.Rel(PathBrowse, url.Path)
		if err != nil {
			return nil, errors.Join(fmt.Errorf("could not get relative path: %w", err), webhandler.ErrBadRequest)
		}
		path = filepath.Clean(path)

		return newFolderDialog{
			url:  r.URL.Path,
			path: path,
		}, nil

	case http.MethodPost:
		base := r.FormValue(newFolderDialogBaseID)
		path := r.FormValue(newFolderDialogPathID)
		if base == "" || path == "" {
			w.WriteHeader(http.StatusUnauthorized)
			return dialogError{err: fmt.Errorf("invalid empty path: %w", webhandler.ErrBadRequest)}, nil
		}
		path = filepath.Clean(filepath.Join(base, path))

		session, err := web.files.Sesssions().Get(auth.Username)
		if err != nil {
			return nil, webhandler.NewInternalError(err, nil)
		}

		if err := hackpadfs.MkdirAll(session.FS(), path, files.DirPerm); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return dialogError{err: fmt.Errorf("error while creating folder %q: %w", path, err)}, nil
		}

		return nil, webhandler.NewRedirectError(PathBrowseAt(path)+sep, http.StatusFound)
	}

	return nil, fmt.Errorf("invalid method %q: %w", r.Method, webhandler.ErrBadRequest)
}

func (web *Web) BrowseDialogUpload(w http.ResponseWriter, r *http.Request) (ui.Component, error) {
	switch r.Method {
	case http.MethodGet:
		return uploadDialog{}, nil
	}

	return nil, fmt.Errorf("invalid method %q: %w", r.Method, webhandler.ErrBadRequest)
}

const (
	newFolderDialogErrorContainerID = "newfolder-error"
	newFolderDialogPathID           = "path"
	newFolderDialogBaseID           = "base"
)

type newFolderDialog struct {
	url  string
	path string
}

var newFolderStyle = ui.MustParseStyle(`
	max-width: var(--size-content-3);
`)
var newFolderFormStyle = ui.MustParseStyle(`
	display: flex;
	flex-direction: column;
	justify-content: center;

	& .hidden {
	  display: none;
	}

	& .buttons {
	  width: 100%;
	  display: flex;
		flex-direction: row;
	  justify-content: center;
	}

	& .error {
		margin: var(--size-3) 0;
	}
`)

func (nfd newFolderDialog) Render(ctx ui.Context) g.Node {
	return h.Div(ctx.Class(newFolderStyle),
		h.H3(g.Text("New Folder")),
		h.P(g.Text("Create a new folder in the currect directory.")),
		h.Br(),
		h.P(
			g.Text("Adding folders recursively is also supported. Thus, creating a folder named "),
			h.Code(g.Text("documents/letters")),
			g.Text(" will create both the "), h.Code(g.Text("documents")),
			g.Text(" and "), h.Code(g.Text("documents/letters")), g.Text(" folders, as necessary."),
		),
		h.Br(),
		h.P(g.Text("Think of this as "), h.Code(g.Text("mkdir -p")), g.Text(".")),

		h.Form(
			ctx.Class(newFolderFormStyle),
			hx.Ext("response-targets"),
			hx.Post(nfd.url),
			hx.Swap("innerHTML"),
			g.Attr("hx-target-error", "#"+newFolderDialogErrorContainerID),

			h.Input(h.Class("hidden"),
				h.Type("text"),
				h.Name(newFolderDialogBaseID),
				h.ID(newFolderDialogBaseID),
				h.Value(nfd.path),
			),
			components.Input(ctx, newFolderDialogPathID, "text", "Path"),

			h.Div(h.Class("buttons"),
				components.Button(ctx, h.Type("submit"), g.Text("Create")),
			),

			h.Div(h.ID(newFolderDialogErrorContainerID), h.Class("error")),
		),
	)
}

type uploadDialog struct {
}

func (ud uploadDialog) Render(ctx ui.Context) g.Node {
	return nil
}

type dialogError struct {
	err error
}

func (de dialogError) Render(ctx ui.Context) g.Node {
	return components.ErrorNotification(ctx, de.err)
}
