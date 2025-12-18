package webhandler

import (
	flag "github.com/spf13/pflag"

	"github.com/teapotovh/teapot/lib/httphandler"
	"github.com/teapotovh/teapot/lib/ui"
)

func WebHandlerFlagSet() (*flag.FlagSet, func() WebHandlerConfig) {
	fs := flag.NewFlagSet("webhandler", flag.ExitOnError)

	httpHandlerFS, getHTTPHandlerConfig := httphandler.HTTPHandlerFlagSet()
	fs.AddFlagSet(httpHandlerFS)

	rendererFS, getRendererConfig := ui.RendererFlagSet()
	fs.AddFlagSet(rendererFS)

	return fs, func() WebHandlerConfig {
		return WebHandlerConfig{
			HTTPHandler: getHTTPHandlerConfig(),
			Renderer:    getRendererConfig(),
		}
	}
}
