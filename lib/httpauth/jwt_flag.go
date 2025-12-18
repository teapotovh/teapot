package httpauth

import (
	"time"

	flag "github.com/spf13/pflag"
)

func JWTAuthFlagSet(defaultIssuer string) (*flag.FlagSet, func() JWTAuthConfig) {
	fs := flag.NewFlagSet("httpauth/jwt", flag.ExitOnError)

	secret := fs.String("httpauth-jwt-secret", "", "the secret used to sign the JWTs")
	issuer := fs.String("httpauth-jwt-issuer", defaultIssuer, "the issuer for the JWTs (the application name)")
	duration := fs.Duration("httpauth-jwt-duration", time.Hour*48, "the duration/expiry of each JWT")

	return fs, func() JWTAuthConfig {
		return JWTAuthConfig{
			Secret:   *secret,
			Issuser:  *issuer,
			Duration: *duration,
		}
	}
}
