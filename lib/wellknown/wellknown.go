package wellknown

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

const (
	Prefix    = "/.well-known/"
	Separator = "/"
)

type Service string

const (
	Caldav Service = "caldav"
)

type Resolver func(r *http.Request) (string, error)

type WellKnown map[Service]Resolver

// ServeHTTP implements http.Handler.
func (h WellKnown) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	if !strings.HasPrefix(path, Prefix) {
		http.NotFound(w, r)
		return
	}

	suffix := strings.Replace(path, Prefix, "", 1)

	parts := strings.Split(suffix, "/")
	if len(parts) <= 0 || len(parts[0]) <= 0 {
		http.NotFound(w, r)
		return
	}

	service := Service(strings.ToLower(parts[0]))

	resolver, ok := h[service]
	if !ok {
		http.NotFound(w, r)
		return
	}

	path, err := resolver(r)
	if err != nil {
		err = fmt.Errorf("computing well-known path for service %q: %w", service, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)

		return
	}

	http.Redirect(w, r, path, http.StatusPermanentRedirect)
}

func ResolverHost(r *http.Request) (string, error) {
	url := url.URL{
		Scheme: r.URL.Scheme,
		Host:   r.Host,
	}

	return url.String(), nil
}

// Ensure ResolverSame implements Resolver.
var _ Resolver = ResolverHost
