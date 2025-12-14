package ldap

import (
	"fmt"

	"github.com/go-ldap/ldap/v3"
)

func (c *Client) list() ([]*ldap.Entry, error) {
	filter, err := c.usersFilter.Render(filterTemplateValues{Username: "*"})
	if err != nil {
		return nil, fmt.Errorf("error while rendering user filter template: %w", err)
	}

	searchRequest := ldap.NewSearchRequest(
		c.usersDN,
		ldap.ScopeWholeSubtree,
		ldap.NeverDerefAliases,
		0,
		0,
		false,
		filter,
		[]string{},
		nil,
	)

	search, err := c.conn.Search(searchRequest)
	if err != nil {
		return nil, fmt.Errorf("error while performing search for user: %w", err)
	}

	var dns []string
	for _, entry := range search.Entries {
		dns = append(dns, entry.DN)
	}

	c.logger.DebugContext(c.ctx, "found user entries", "dns", dns)

	return search.Entries, nil
}

func (c *Client) find(username string) (*ldap.Entry, error) {
	filter, err := c.usersFilter.Render(filterTemplateValues{
		Username: username,
	})
	if err != nil {
		return nil, fmt.Errorf("error while rendering user filter template: %w", err)
	}

	searchRequest := ldap.NewSearchRequest(
		c.usersDN,
		ldap.ScopeWholeSubtree,
		ldap.NeverDerefAliases,
		0,
		0,
		false,
		filter,
		[]string{},
		nil,
	)

	search, err := c.conn.Search(searchRequest)
	if err != nil {
		return nil, fmt.Errorf("error while performing search for user: %w", err)
	}

	if len(search.Entries) == 0 {
		return nil, ErrUserNotFound
	}

	if len(search.Entries) > 1 {
		return nil, ErrTooManyMatches
	}

	entry := search.Entries[0]

	cn := entry.GetAttributeValue("cn")
	if username != cn {
		return nil, fmt.Errorf("usernames don't match, expected %s, got %s", username, cn)
	}

	log := c.logger.With("username", username)
	for _, attr := range entry.Attributes {
		log = log.With(attr.Name, attr.Values)
	}

	log.DebugContext(c.ctx, "found user entry for username")

	return entry, nil
}
