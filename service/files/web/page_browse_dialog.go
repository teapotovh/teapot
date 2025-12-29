package web

import (
	"errors"
	"fmt"
	"io"
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

// Extracts the base path for a dialog from the HX-Current-URL header
// and trims it down by removing the prefix
func getDialogBasePath(r *http.Request, prefix string) (string, error) {
	curr := hxhttp.GetCurrentURL(r.Header)
	url, err := url.Parse(curr)
	if err != nil {
		return "", errors.Join(
			fmt.Errorf("could not parse current URL %q : %w", curr, err),
			webhandler.ErrBadRequest,
		)
	}

	path, err := filepath.Rel(prefix, url.Path)
	if err != nil {
		return "", errors.Join(fmt.Errorf("could not get relative path: %w", err), webhandler.ErrBadRequest)
	}

	return filepath.Clean(path), nil
}

func (web *Web) BrowseDialogNewFolder(w http.ResponseWriter, r *http.Request) (ui.Component, error) {
	auth := webauth.GetAuth(r)
	if auth == nil {
		return nil, webhandler.NewRedirectError(PathIndex, http.StatusFound)
	}

	switch r.Method {
	case http.MethodGet:
		path, err := getDialogBasePath(r, PathBrowse)
		if err != nil {
			return nil, err
		}

		return newFolderDialog{
			url:  r.URL.Path,
			path: path,
		}, nil

	case http.MethodPost:
		base := r.FormValue(dialogBaseID)
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
	auth := webauth.GetAuth(r)
	if auth == nil {
		return nil, webhandler.NewRedirectError(PathIndex, http.StatusFound)
	}

	switch r.Method {
	case http.MethodGet:
		path, err := getDialogBasePath(r, PathBrowse)
		if err != nil {
			return nil, err
		}

		return uploadDialog{
			url:  r.URL.Path,
			path: path,
		}, nil

	case http.MethodPost:
		if err := r.ParseMultipartForm(files.MaxSize); err != nil {
			return nil, webhandler.NewInternalError(err, nil)
		}

		base := r.FormValue(dialogBaseID)
		file, header, _ := r.FormFile(uploadDialogFileID)
		if base == "" || file == nil || header == nil || header.Filename == "" || header.Size <= 0 {
			web.logger.Info("got file", "f", file)
			w.WriteHeader(http.StatusUnauthorized)
			return dialogError{err: fmt.Errorf("invalid empty path/file: %w", webhandler.ErrBadRequest)}, nil
		}

		path := filepath.Clean(filepath.Join(base, header.Filename))

		session, err := web.files.Sesssions().Get(auth.Username)
		if err != nil {
			return nil, webhandler.NewInternalError(err, nil)
		}

		bytes, err := io.ReadAll(file)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return dialogError{err: fmt.Errorf("could not read file from request: %w", err)}, nil
		}

		if err := hackpadfs.WriteFullFile(session.FS(), path, bytes, files.DirPerm); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return dialogError{err: fmt.Errorf("error while writing file %q: %w", path, err)}, nil
		}

		return nil, webhandler.NewRedirectError(PathBrowseAt(base)+sep, http.StatusFound)
	}

	return nil, fmt.Errorf("invalid method %q: %w", r.Method, webhandler.ErrBadRequest)
}

const (
	newFolderDialogErrorContainerID = "newfolder-error"
	dialogBaseID                    = "base"
	newFolderDialogPathID           = "path"
	uploadDialogFileID              = "file"
)

var dialogStyle = ui.MustParseStyle(`
	max-width: var(--size-content-3);
`)

type newFolderDialog struct {
	url  string
	path string
}

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

	& .input {
    margin: var(--size-6) 0;
	}

	& .error {
		margin: var(--size-3) 0;
	}
`)

func (nfd newFolderDialog) Render(ctx ui.Context) g.Node {
	return h.Div(ctx.Class(dialogStyle),
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
				h.Name(dialogBaseID),
				h.ID(dialogBaseID),
				h.Value(nfd.path),
			),
			components.Input(ctx, newFolderDialogPathID, "text", "Path", h.Class("input")),

			h.Div(h.Class("buttons"),
				components.Button(ctx, h.Type("submit"), g.Text("Create")),
			),

			h.Div(h.ID(newFolderDialogErrorContainerID), h.Class("error")),
		),
	)
}

type uploadDialog struct {
	url  string
	path string
}

var uploadFormStyle = ui.MustParseStyle(`
	display: flex;
	flex-direction: row;
	justify-content: center;

	& .hidden {
	  display: none;
	}

	& .input {
	  width: 100%;
		margin: var(--size-7) 0;

	  display: flex;
		flex-direction: row;
	  justify-content: center;
	}

	& .error {
		margin: var(--size-3) 0;
	}
`)

func (ud uploadDialog) Render(ctx ui.Context) g.Node {
	cd := filepath.Base(ud.path)
	exampleFile := "main.cpp"
	return h.Div(ctx.Class(dialogStyle),
		h.H3(g.Text("Upload")),
		h.P(g.Text("Upload a file in the current directory.")),
		h.Br(),
		h.P(
			g.Text("The file will maintain the same name it has when you upload it."),
			g.Text("For example, if you upload a faile called"),
			h.Code(g.Text(exampleFile)),
			g.Text(" it will be placed in the current folder ("), h.Code(g.Text(cd)),
			g.Text(") as "), h.Code(g.Text(exampleFile)), g.Text("."),
		),
		h.Br(),
		g.Text("Placing files under subfolders is not supported, although you could get it to work with some trickery ;)."),

		h.Form(
			ctx.Class(uploadFormStyle),
			hx.Ext("response-targets"),
			hx.Encoding("multipart/form-data"),
			hx.Post(ud.url),
			hx.Swap("innerHTML"),
			g.Attr("hx-target-error", "#"+newFolderDialogErrorContainerID),

			h.Input(h.Class("hidden"),
				h.Type("text"),
				h.Name(dialogBaseID),
				h.ID(dialogBaseID),
				h.Value(ud.path),
			),

			h.Div(h.Class("input"),
				components.FileInput(ctx, uploadDialogFileID, "Select a file",
					g.Attr("onchange", "document.querySelector('label[for="+uploadDialogFileID+"]').textContent = this.files[0]?.name || 'Select a file'"),
				),
				components.Button(ctx, h.Type("submit"), g.Text("Upload")),
			),

			h.Div(h.ID(newFolderDialogErrorContainerID), h.Class("error")),
		),
	)
}

type dialogError struct {
	err error
}

func (de dialogError) Render(ctx ui.Context) g.Node {
	return components.ErrorNotification(ctx, de.err)
}
