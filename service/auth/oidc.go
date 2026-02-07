package auth

import (
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/zitadel/oidc/v3/pkg/op"
	"golang.org/x/text/language"
)

var ErrInvalidKeyLength = errors.New("invalid key length")

type oidcConfig struct {
	key      []byte
	duration time.Duration
}

func parseOIDCKey(key []byte) (out [32]byte, err error) {
	if len(key) != len(out) {
		return out, fmt.Errorf("got %d bytes, wanted %d: %w", len(key), len(out), ErrInvalidKeyLength)
	}

	copy(out[:], key)
	return out, nil
}

func newOIDCProvider(config oidcConfig, logger *slog.Logger) (*op.Provider, error) {
	cryptoKey, err := parseOIDCKey(config.key)
	if err != nil {
		return nil, fmt.Errorf("error while parsing cryptographic key from hex string %q: %w", config.key, err)
	}

	cfg := op.Config{
		CryptoKey:                cryptoKey,
		DefaultLogoutRedirectURI: PathLogout,

		CodeMethodS256:          true,
		AuthMethodPost:          true,
		AuthMethodPrivateKeyJWT: true,
		GrantTypeRefreshToken:   true,
		RequestObjectSupported:  true,

		SupportedUILocales: []language.Tag{language.English},

		DeviceAuthorization: op.DeviceAuthorizationConfig{
			Lifetime:     config.duration,
			PollInterval: config.duration,
			UserFormPath: PathDevice,
			UserCode:     op.UserCodeBase20,
		},
	}

	var opts = []op.Option{
		op.WithLogger(logger),
		op.WithAllowInsecure(),
	}

	opts = append(opts)

	var storage op.Storage
	var issuer func(insecure bool) (op.IssuerFromRequest, error)

	handler, err := op.NewProvider(&cfg, storage, issuer, opts...)
	if err != nil {
		return nil, fmt.Errorf("error while constructing provider: %w", err)
	}

	return handler, err
}
