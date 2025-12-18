package ui

import (
	flag "github.com/spf13/pflag"
)

func RendererFlagSet() (*flag.FlagSet, func() RendererConfig) {
	fs := flag.NewFlagSet("ui/renderer", flag.ExitOnError)

	assetPath := fs.String("ui-renderer-asset-path", "/static/", "the URI path where assets will be served")

	return fs, func() RendererConfig {
		return RendererConfig{
			AssetPath: *assetPath,
		}
	}
}
