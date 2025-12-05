package ui

import (
	flag "github.com/spf13/pflag"
)

type UIConfig struct {
	Renderer RendererConfig
}

func UIFlagSet() (*flag.FlagSet, func() UIConfig) {
	fs := flag.NewFlagSet("ui", flag.ExitOnError)

	rendererFS, getRendererConfig := RendererFlagSet()
	fs.AddFlagSet(rendererFS)

	return fs, func() UIConfig {
		return UIConfig{
			Renderer: getRendererConfig(),
		}
	}
}
