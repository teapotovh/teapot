package ldap

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/go-ldap/ldap/v3"

	"github.com/teapotovh/teapot/lib/tmplstring"
)

var (
	ErrUserNotFound       = errors.New("user not found")
	ErrTooManyMatches     = errors.New("too many matches for user search")
	ErrInvalidCredentials = errors.New("invalid credentials")
)

type filterTemplateValues struct {
	Username string
}

// Client holds a connection to an LDAP server and can be used to perform
// high-level user-management instructions on that server.
type Client struct {
	logger *slog.Logger

	ctx  context.Context
	conn *ldap.Conn

	usersDN      string
	usersFilter  *tmplstring.TMPL[filterTemplateValues]
	groupsDN     string
	adminGroupDN string
	accessesDN   string
}

func (c *Client) Authenticate(username string, password string) (*User, error) {
	entry, err := c.find(username)
	if err != nil {
		return nil, fmt.Errorf("error while looking up user for bind: %w", err)
	}

	if err := c.conn.Bind(entry.DN, password); err != nil {
		return nil, fmt.Errorf("error while binding as %s: %w, likely %w", entry.DN, err, ErrInvalidCredentials)
	}

	return c.mapUser(entry)
}

func (c *Client) Close() {
	if err := c.conn.Close(); err != nil {
		c.logger.ErrorContext(c.ctx, "error while closing LDAP client", "err", err)
	}
}
