package openprops

import "embed"

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

//go:generate go run ./download/
//go:embed css/*.css
var CSS embed.FS
