package htmx

import (
	"embed"
	"fmt"
	"strings"

	"github.com/teapotovh/teapot/lib/ui/dependency"
)

var Components = []string{
	"htmx.org",
	"htmx-ext-response-targets",
}

//go:generate go run ./download/
//go:embed js/*.js
var JS embed.FS

func Dependencies() (map[dependency.Dependency][]byte, error) {
	result := map[dependency.Dependency][]byte{}

	for i, component := range Components {
		var name string
		if i == 0 {
			name = "htmx"
		} else {
			name = strings.TrimPrefix(component, "htmx-ext-")
		}

		dep := dependency.Dependency{
			Type: dependency.DependencyTypeScript,
			Name: name,
		}

		path := fmt.Sprintf("js/%s.js", name)

		bytes, err := JS.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("error while loading file from bundle at %q for dependency %q: %w", path, dep, err)
		}

		result[dep] = bytes
	}

	return result, nil
}
