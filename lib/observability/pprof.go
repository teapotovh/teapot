package observability

import (
	"log/slog"
	"net/http"
	"net/http/pprof"
)

// Until is fixed: https://github.com/golang/go/issues/42834
// the pprof package pollutes the net/http's DefaultServeMux, so we reset it
// here for safety.

//nolint:gochecknoinits
func init() {
	http.DefaultServeMux = http.NewServeMux()
}

type httpServicePProf struct {
	logger *slog.Logger
}

func (p *httpServicePProf) wrapHandler(handler http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p.logger.Warn("calling pprof debug endpoint", "path", r.URL.Path, "query", r.URL.Query())

		handler(w, r)
	})
}

// Handler implements httpsrv.Handler.
func (p *httpServicePProf) Handler(prefix string) http.Handler {
	mux := http.NewServeMux()

	mux.Handle(prefix+"/pprof/", p.wrapHandler(pprof.Index))
	mux.Handle(prefix+"/pprof/cmdline", p.wrapHandler(pprof.Cmdline))
	mux.Handle(prefix+"/pprof/profile", p.wrapHandler(pprof.Profile))
	mux.Handle(prefix+"/pprof/symbol", p.wrapHandler(pprof.Symbol))
	mux.Handle(prefix+"/pprof/trace", p.wrapHandler(pprof.Trace))

	return mux
}
