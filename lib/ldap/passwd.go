package ldap

import (
	"errors"
	"fmt"

	"github.com/go-ldap/ldap/v3"
)

var (
	ErrUnexpectedGeneratedPassword = errors.New("LDAP server unexpectedly returned a generated password")
)

func (c *Client) Passwd(username, password string) error {
	entry, err := c.find(username)
	if err != nil {
		return fmt.Errorf("error while looking up user: %w", err)
	}

	passwordModifyRequest := ldap.NewPasswordModifyRequest(
		entry.DN,
		"",
		password,
	)
	res, err := c.conn.PasswordModify(passwordModifyRequest)
	if err != nil {
		return fmt.Errorf("error while modifying user password: %w", err)
	}
	if res.GeneratedPassword != "" {
		return ErrUnexpectedGeneratedPassword
	}

	return nil
}
