package webauth

import (
	flag "github.com/spf13/pflag"

	"github.com/teapotovh/teapot/lib/httpauth"
)

func WebAuthFlagSet(defaultIssuer string) (*flag.FlagSet, func() WebAuthConfig) {
	fs := flag.NewFlagSet("webauth", flag.ExitOnError)

	resetPasswordURL := fs.String(
		"webauth-reset-password-url",
		"https://user.teapot.ovh/recover",
		"the URL where a user can recover their password",
	)
	jwtAuthFS, getJWTAuthConfig := httpauth.JWTAuthFlagSet(defaultIssuer)
	fs.AddFlagSet(jwtAuthFS)

	return fs, func() WebAuthConfig {
		return WebAuthConfig{
			JWTAuth:          getJWTAuthConfig(),
			ResetPasswordURL: *resetPasswordURL,
		}
	}
}
