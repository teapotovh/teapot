package web

import (
	flag "github.com/spf13/pflag"

	"github.com/teapotovh/teapot/lib/httpauth"
	"github.com/teapotovh/teapot/lib/ui"
)

func WebFlagSet() (*flag.FlagSet, func() WebConfig) {
	fs := flag.NewFlagSet("files/web", flag.ExitOnError)

	uiFS, getUIConfig := ui.UIFlagSet()
	fs.AddFlagSet(uiFS)

	jwtAuthFS, getJWTAuthConfig := httpauth.JWTAuthFlagSet("files/web")
	fs.AddFlagSet(jwtAuthFS)

	return fs, func() WebConfig {
		return WebConfig{
			UI:      getUIConfig(),
			JWTAuth: getJWTAuthConfig(),
		}
	}
}
