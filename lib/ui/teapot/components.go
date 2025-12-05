package teapot

import (
	"embed"
	"fmt"

	"github.com/teapotovh/teapot/lib/ui/dependency"
)

var (
	CSSComponents = []string{
		"theme",
	}
	JSComponents = []string{
		"render",
	}
)

//go:embed css/*.css
var CSS embed.FS

//go:embed js/*.js
var JS embed.FS

func Dependencies() (map[dependency.Dependency][]byte, error) {
	result := map[dependency.Dependency][]byte{}
	for _, component := range CSSComponents {
		dep := dependency.Dependency{
			Type: dependency.DependencyTypeStyle,
			Name: component,
		}

		path := fmt.Sprintf("css/%s.css", component)
		bytes, err := CSS.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("error while loading file from bundle at %q for dependency %q: %w", path, dep, err)
		}

		result[dep] = bytes
	}

	for _, component := range JSComponents {
		dep := dependency.Dependency{
			Type: dependency.DependencyTypeScript,
			Name: component,
		}

		path := fmt.Sprintf("js/%s.js", component)
		bytes, err := JS.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("error while loading file from bundle at %q for dependency %q: %w", path, dep, err)
		}

		result[dep] = bytes
	}

	return result, nil
}
