package web

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"

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
	BrowseDialogRename    = "rename"
	BrowseDialogDelete    = "delete"
)

var ErrInvalidBrowseDialog = errors.New("invalid browse dialog")

func (web *Web) BrowseDialog(w http.ResponseWriter, r *http.Request) (ui.Component, error) {
	dialog := r.PathValue("dialog")
	switch dialog {
	case BrowseDialogNewFolder:
		return web.BrowseDialogNewFolder(w, r)

	case BrowseDialogUpload:
		return web.BrowseDialogUpload(w, r)

	case BrowseDialogRename:
		return web.BrowseDialogRename(w, r)

	case BrowseDialogDelete:
		return web.BrowseDialogDelete(w, r)

	default:
		return nil, fmt.Errorf("could not serve requested dialog %q: %w", dialog, ErrInvalidBrowseDialog)
	}
}

// Extracts the base path for a dialog from the HX-Current-URL header
// and trims it down by removing the prefix.
func getDialogBasePath(r *http.Request, prefix string) (string, error) {
	curr := hxhttp.GetCurrentURL(r.Header)

	u, err := url.Parse(curr)
	if err != nil {
		return "", errors.Join(
			fmt.Errorf("could not parse current URL %q : %w", curr, err),
			webhandler.ErrBadRequest,
		)
	}

	dir, err := urlPath(prefix, u.Path)
	if err != nil {
		return "", fmt.Errorf("could not get relative path: %w", err)
	}

	return dir, nil
}

func (web *Web) BrowseDialogNewFolder(w http.ResponseWriter, r *http.Request) (ui.Component, error) {
	auth := webauth.GetAuth(r)
	if auth == nil {
		return nil, webhandler.NewRedirectError(PathIndex, http.StatusFound)
	}

	switch r.Method {
	case http.MethodGet:
		dir, err := getDialogBasePath(r, PathBrowse)
		if err != nil {
			return nil, err
		}

		return newFolderDialog{
			url:  EscapePath(r.URL.Path),
			path: dir,
		}, nil

	case http.MethodPost:
		base := r.FormValue(dialogBaseID)

		rel := r.FormValue(newFolderDialogPathID)
		if base == "" || rel == "" {
			w.WriteHeader(http.StatusUnauthorized)
			return dialogError{err: fmt.Errorf("invalid empty path: %w", webhandler.ErrBadRequest)}, nil
		}

		dir := path.Join(base, rel)

		session, err := web.files.Session(r.Context(), auth.Username)
		if err != nil {
			return nil, webhandler.NewInternalError(err, nil)
		}

		if err := session.FS().Mkdir(r.Context(), storePath(dir)); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return dialogError{err: fmt.Errorf("error while creating folder %q: %w", dir, err)}, nil
		}

		return nil, webhandler.NewRedirectError(EscapePath(PathBrowseDirAt(dir)), http.StatusFound)
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
		dir, err := getDialogBasePath(r, PathBrowse)
		if err != nil {
			return nil, err
		}

		return uploadDialog{
			url:  EscapePath(r.URL.Path),
			path: dir,
		}, nil

	case http.MethodPost:
		if err := r.ParseMultipartForm(files.MaxSize); err != nil { //nolint:gosec
			return nil, webhandler.NewInternalError(err, nil)
		}

		base := r.FormValue(dialogBaseID)

		file, header, _ := r.FormFile(uploadDialogFileID)
		if base == "" || file == nil || header == nil || header.Filename == "" || header.Size <= 0 {
			w.WriteHeader(http.StatusBadRequest)

			return dialogError{err: fmt.Errorf("invalid empty path/file: %w", webhandler.ErrBadRequest)}, nil
		}

		rel := path.Join(base, header.Filename)

		session, err := web.files.Session(r.Context(), auth.Username)
		if err != nil {
			return nil, webhandler.NewInternalError(err, nil)
		}

		data, err := io.ReadAll(file)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return dialogError{err: fmt.Errorf("could not read file from request: %w", err)}, nil
		}

		if err := session.FS().Put(r.Context(), storePath(rel), bytes.NewReader(data)); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return dialogError{err: fmt.Errorf("error while writing file %q: %w", rel, err)}, nil
		}

		return nil, webhandler.NewRedirectError(EscapePath(PathBrowseDirAt(base)), http.StatusFound)
	}

	return nil, fmt.Errorf("invalid method %q: %w", r.Method, webhandler.ErrBadRequest)
}

func (web *Web) BrowseDialogRename(w http.ResponseWriter, r *http.Request) (ui.Component, error) {
	auth := webauth.GetAuth(r)
	if auth == nil {
		return nil, webhandler.NewRedirectError(PathIndex, http.StatusFound)
	}

	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		return nil, fmt.Errorf("invalid method %q: %w", r.Method, webhandler.ErrBadRequest)
	}

	base, err := getDialogBasePath(r, PathBrowse)
	if err != nil {
		return nil, err
	}

	paths, err := selectedPaths(r)
	if err != nil {
		return nil, err
	}

	if len(paths) != 1 {
		return nil, fmt.Errorf("rename requires one entry: %w", webhandler.ErrBadRequest)
	}

	return renameDialog{
		url:  EscapePath(PathRenameAt(path.Join(base, paths[0]))),
		path: paths[0],
	}, nil
}

func (web *Web) BrowseDialogDelete(w http.ResponseWriter, r *http.Request) (ui.Component, error) {
	auth := webauth.GetAuth(r)
	if auth == nil {
		return nil, webhandler.NewRedirectError(PathIndex, http.StatusFound)
	}

	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		return nil, fmt.Errorf("invalid method %q: %w", r.Method, webhandler.ErrBadRequest)
	}

	path, err := getDialogBasePath(r, PathBrowse)
	if err != nil {
		return nil, err
	}

	paths, err := selectedPaths(r)
	if err != nil {
		return nil, err
	}

	return deleteDialog{
		url:   EscapePath(PathDeleteDirAt(path)),
		paths: paths,
	}, nil
}

const (
	newFolderDialogErrorContainerID = "newfolder-error"
	dialogBaseID                    = "base"
	newFolderDialogPathID           = "path"
	uploadDialogFileID              = "file"
	renameDialogNameID              = "name"
)

var dialogStyle = ui.MustParseStyle(`
	max-width: calc(var(--size-content-3) + var(--size-10));

	& h3 {
	  margin-bottom: var(--size-3);
	}

	& p {
	  margin-bottom: var(--size-3);
	}

	& ul {
	  margin-bottom: var(--size-3);
	  padding-left: var(--size-5);
	}

	& form {
	  margin-top: var(--size-3);
	}
`)

var renameDialogStyle = ui.MustParseStyle(`
	width: calc(var(--size-content-3) + var(--size-10));
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
		h.P(
			g.Text("Adding folders recursively is also supported. Thus, creating a folder named "),
			components.Code(ctx, g.Text("documents/letters")),
			g.Text(" will create both the "), components.Code(ctx, g.Text("documents")),
			g.Text(" and "), components.Code(ctx, g.Text("documents/letters")), g.Text(" folders, as necessary."),
		),
		h.P(g.Text("Think of this as "), components.Code(ctx, g.Text("mkdir -p")), g.Text(".")),

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
	cd := path.Base(ud.path)
	exampleFile := "main.cpp"

	return h.Div(
		ctx.Class(dialogStyle),
		h.H3(g.Text("Upload")),
		h.P(g.Text("Upload a file in the current directory.")),
		h.P(
			g.Text("The file will maintain the same name it has when you upload it."),
			g.Text("For example, if you upload a faile called"),
			components.Code(ctx, g.Text(exampleFile)),
			g.Text(" it will be placed in the current folder ("), components.Code(ctx, g.Text(cd)),
			g.Text(") as "), components.Code(ctx, g.Text(exampleFile)), g.Text("."),
		),
		h.P(
			g.Text(
				"Placing files under subfolders is not supported, although you could get it to work with some trickery ;).",
			),
		),

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
				components.FileInput(
					ctx,
					uploadDialogFileID,
					"Select a file",
					g.Attr(
						"onchange",
						"document.querySelector('label[for="+uploadDialogFileID+"]').textContent = this.files[0]?.name || 'Select a file'",
					),
				),
				components.Button(ctx, h.Type("submit"), g.Text("Upload")),
			),

			h.Div(h.ID(newFolderDialogErrorContainerID), h.Class("error")),
		),
	)
}

type renameDialog struct {
	url  string
	path string
}

func (rd renameDialog) Render(ctx ui.Context) g.Node {
	name := path.Base(rd.path)

	return h.Div(ctx.Class(dialogStyle, renameDialogStyle),
		h.H3(g.Text("Rename")),
		h.P(g.Text("Rename the selected entry.")),
		h.P(components.Code(ctx, g.Text(name))),

		h.Form(
			ctx.Class(newFolderFormStyle),
			hx.Ext("response-targets"),
			hx.Post(rd.url),
			hx.Swap("innerHTML"),
			g.Attr("hx-target-error", "#"+newFolderDialogErrorContainerID),

			components.ValueInput(ctx, renameDialogNameID, "text", "Name", name, true, h.Class("input")),

			h.Div(h.Class("buttons"),
				components.Button(ctx, h.Type("submit"), g.Text("Rename")),
			),

			h.Div(h.ID(newFolderDialogErrorContainerID), h.Class("error")),
		),
	)
}

type deleteDialog struct {
	url   string
	paths []string
}

func (dd deleteDialog) Render(ctx ui.Context) g.Node {
	return h.Div(ctx.Class(dialogStyle),
		h.H3(g.Text("Delete")),
		h.P(g.Text("Delete the selected entries? This cannot be undone.")),
		h.Ul(
			g.Map(dd.paths, func(filePath string) g.Node {
				return h.Li(components.Code(ctx, g.Text(path.Base(filePath))))
			}),
		),

		h.Form(
			ctx.Class(newFolderFormStyle),
			hx.Ext("response-targets"),
			hx.Post(dd.url),
			hx.Swap("innerHTML"),
			g.Attr("hx-target-error", "#"+newFolderDialogErrorContainerID),

			g.Map(dd.paths, func(path string) g.Node {
				return h.Input(
					h.Type("hidden"),
					h.Name(selectionPathID),
					h.Value(path),
				)
			}),

			h.Div(h.Class("buttons"),
				components.Button(ctx, h.Type("submit"), g.Text("Delete")),
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
