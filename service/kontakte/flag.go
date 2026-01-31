package kontakte

import (
	flag "github.com/spf13/pflag"

	"github.com/teapotovh/teapot/lib/httplog"
	"github.com/teapotovh/teapot/lib/ldap"
	"github.com/teapotovh/teapot/lib/webauth"
	"github.com/teapotovh/teapot/lib/webhandler"
)

func KontakteFlagSet() (*flag.FlagSet, func() KontakteConfig) {
	fs := flag.NewFlagSet("kontakte", flag.ExitOnError)

	httpLogFS, getHTTPLogConfig := httplog.HTTPLogFlagSet()
	fs.AddFlagSet(httpLogFS)

	webHandlerFS, getWebHandlerConfig := webhandler.WebHandlerFlagSet()
	fs.AddFlagSet(webHandlerFS)

	ldapFS, getLDAPConfig := ldap.LDAPFlagSet()
	fs.AddFlagSet(ldapFS)

	webAuthFS, getWebAuthConfig := webauth.WebAuthFlagSet("kontakte")
	fs.AddFlagSet(webAuthFS)

	return fs, func() KontakteConfig {
		return KontakteConfig{
			HTTPLog:    getHTTPLogConfig(),
			WebHandler: getWebHandlerConfig(),
			LDAP:       getLDAPConfig(),
			WebAuth:    getWebAuthConfig(),
		}
	}
}
