package kontakte

import (
	"net/http"

	g "maragu.dev/gomponents"
	hxhttp "maragu.dev/gomponents-htmx/http"

	_ "embed"
)

//go:embed style.css
var styleCSS []byte

func (srv *Server) HandleStyle(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/css")
	w.Write(styleCSS) //nolint:all
}

func Redirect(w http.ResponseWriter, r *http.Request, url string) {
	if hxhttp.IsRequest(r.Header) {
		hxhttp.SetLocation(w.Header(), url)
	} else {
		http.Redirect(w, r, url, http.StatusFound)
	}
}

func (srv *Server) HandleNotFound(w http.ResponseWriter, r *http.Request) (g.Node, error) {
	return NotFound(r)
}

func (srv *Server) HandleIndex(w http.ResponseWriter, r *http.Request) {
	auth := getAuth(r.Context())
	if auth != nil {
		if auth.Admin {
			Redirect(w, r, PathUsers)
		} else {
			Redirect(w, r, PathUser(auth.Subject))
		}
	} else {
		Redirect(w, r, PathLogin)
	}
}
