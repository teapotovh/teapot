package openprops

import (
	"embed"
	"fmt"

	"github.com/teapotovh/teapot/lib/ui/dependency"
)

var (
	Components = []string{
		"normalize",
		"animations",
		"aspects",
		"borders",
		"colors",
		"durations",
		"easings",
		"fonts",
		"gradients",
		"masks.edges",
		"masks.corner-cuts",
		"media",
		"shadows",
		"sizes",
		"supports",
		"zindex",
		"brand-colors",
	}
)

var nameMap = map[string]string{
	"sizes": "size",
	"fonts": "font",
}

//go:generate go run ./download/
//go:embed css/*.css
var CSS embed.FS

func Dependencies() (map[dependency.Dependency][]byte, error) {
	result := map[dependency.Dependency][]byte{}
	for _, component := range Components {
		name := component
		if newName, ok := nameMap[component]; ok {
			name = newName
		}
		dep := dependency.Dependency{
			Type: dependency.DependencyTypeStyle,
			Name: name,
		}

		path := fmt.Sprintf("css/%s.css", component)
		bytes, err := CSS.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("error while loading file from bundle at %q for dependency %q: %w", path, dep, err)
		}

		result[dep] = bytes
	}

	return result, nil
}
