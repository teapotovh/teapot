package auth

import (
	"time"

	flag "github.com/spf13/pflag"

	"github.com/teapotovh/teapot/lib/httplog"
	"github.com/teapotovh/teapot/lib/ldap"
	"github.com/teapotovh/teapot/lib/webauth"
	"github.com/teapotovh/teapot/lib/webhandler"
)

var DefaultKey []byte

func init() {
	for i := range 32 {
		DefaultKey = append(DefaultKey, byte(i))
	}
}

func AuthFlagSet() (*flag.FlagSet, func() AuthConfig) {
	fs := flag.NewFlagSet("auth", flag.ExitOnError)

	httpLogFS, getHTTPLogConfig := httplog.HTTPLogFlagSet()
	fs.AddFlagSet(httpLogFS)

	webHandlerFS, getWebHandlerConfig := webhandler.WebHandlerFlagSet()
	fs.AddFlagSet(webHandlerFS)

	ldapFS, getLDAPConfig := ldap.LDAPFlagSet()
	fs.AddFlagSet(ldapFS)

	webAuthFS, getWebAuthConfig := webauth.WebAuthFlagSet("kontakte")
	fs.AddFlagSet(webAuthFS)

	key := fs.BytesHex("auth-key", DefaultKey, "the key used to encrypt authentication tokens")
	duration := fs.Duration("auth-duration", time.Hour*24*7, "the duration/expiry of a device authentication")

	return fs, func() AuthConfig {
		return AuthConfig{
			HTTPLog:    getHTTPLogConfig(),
			WebHandler: getWebHandlerConfig(),
			LDAP:       getLDAPConfig(),
			WebAuth:    getWebAuthConfig(),

			Key:      *key,
			Duration: *duration,
		}
	}
}
