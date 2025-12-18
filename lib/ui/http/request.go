package http

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/teapotovh/teapot/lib/ui"
	"github.com/teapotovh/teapot/lib/ui/dependency"
)

const (
	HeaderTeapotStyles       = "X-Teapot-Styles"
	HeaderTeapotDependencies = "X-Teapot-Dependencies"
)

func parseSet[T comparable](raw string, fn func(string) (T, error)) (map[T]ui.Unit, error) {
	var strs []string
	if err := json.Unmarshal([]byte(raw), &strs); err != nil {
		return nil, fmt.Errorf("error while parsing header %q: %w", raw, err)
	}

	result := map[T]ui.Unit{}

	for _, str := range strs {
		key, err := fn(str)
		if err != nil {
			return nil, fmt.Errorf("error while parsing element %q in header: %w", str, err)
		}

		result[key] = ui.Unit{}
	}

	return result, nil
}

func AlreadyLoadedFromRequest(r *http.Request) (ui.AlreadyLoaded, error) {
	styles, err := parseSet(r.Header.Get(HeaderTeapotStyles), func(s string) (string, error) { return s, nil })
	if err != nil {
		return ui.AlreadyLoaded{}, fmt.Errorf(
			"error while parsing already loaded syles from header %q: %w",
			HeaderTeapotStyles,
			err,
		)
	}

	deps, err := parseSet(r.Header.Get(HeaderTeapotStyles), dependency.ParseDependency)
	if err != nil {
		return ui.AlreadyLoaded{}, fmt.Errorf(
			"error while parsing already loaded dependencies from header %q: %w",
			HeaderTeapotDependencies,
			err,
		)
	}

	al := ui.AlreadyLoaded{
		Styles:       styles,
		Dependencies: deps,
	}

	return al, nil
}
