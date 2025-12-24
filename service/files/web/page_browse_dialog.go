package web

import (
	"errors"
	"fmt"
	"net/http"

	g "maragu.dev/gomponents"
	hx "maragu.dev/gomponents-htmx"
	h "maragu.dev/gomponents/html"

	"github.com/kataras/muxie"
	"github.com/teapotovh/teapot/lib/ui"
	"github.com/teapotovh/teapot/lib/ui/components"
	"github.com/teapotovh/teapot/lib/webhandler"
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
	switch r.Method {
	case http.MethodGet:
		return newFolderDialog{
			path: r.URL.Path,
		}, nil
	case http.MethodPost:
		return nil, fmt.Errorf("not implemented yet: %w", webhandler.ErrBadRequest)
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

const newFolderDialogErrorContainerID = "newfolder-error"

type newFolderDialog struct {
	path string
}

var newFolderFormStyle = ui.MustParseStyle(`
	display: flex;
	flex-direction: column;
	justify-content: center;

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
	return h.Div(
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
			hx.Post(nfd.path),
			hx.Swap("innerHTML"),
			g.Attr("hx-target-error", "#"+newFolderDialogErrorContainerID),

			components.Input(ctx, "path", "text", "Path"),
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
