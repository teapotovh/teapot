package ldap

import (
	"context"
	"errors"
	"fmt"

	"github.com/go-ldap/ldap/v3"
	"github.com/teapotovh/teapot/lib/observability"
	"go.opentelemetry.io/otel/attribute"
)

var ErrUnexpectedGeneratedPassword = errors.New("LDAP server unexpectedly returned a generated password")

func (c *Client) Passwd(ctx context.Context, username, password string) (err error) {
	ctx, span := observability.TracerFromContext(ctx).Start(ctx, "Client.Passwd")
	defer observability.SpanEnd(span, err)
	span.SetAttributes(attribute.String("username", username))

	entry, err := c.find(ctx, username)
	if err != nil {
		return fmt.Errorf("error while looking up user: %w", err)
	}

	passwordModifyRequest := ldap.NewPasswordModifyRequest(
		entry.DN,
		"",
		password,
	)

	res, err := passwd(c.metrics, c.conn, passwordModifyRequest)
	if err != nil {
		return fmt.Errorf("error while modifying user password: %w", err)
	}

	if res.GeneratedPassword != "" {
		return ErrUnexpectedGeneratedPassword
	}

	return nil
}
