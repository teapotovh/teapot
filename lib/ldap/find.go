package ldap

import (
	"context"
	"errors"
	"fmt"

	"github.com/go-ldap/ldap/v3"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/teapotovh/teapot/lib/observability"
)

var (
	ErrNotMatchingUsername = errors.New("username doesn't match")
)

func (c *Client) list(ctx context.Context) (entries []*ldap.Entry, err error) {
	ctx, span := observability.TracerFromContext(ctx).Start(ctx, "ldap.list")
	defer observability.SpanEnd(span, err)

	filter, err := c.usersFilter.Render(filterTemplateValues{Username: "*"})
	if err != nil {
		return nil, fmt.Errorf("error while rendering user filter template: %w", err)
	}

	span.AddEvent(
		"rendered find filter template string for all users",
		trace.WithAttributes(attribute.String("filter", filter)),
	)

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

	search, err := search(ctx, c.metrics, c.conn, searchRequest)
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

func (c *Client) find(ctx context.Context, username string) (entry *ldap.Entry, err error) {
	ctx, span := observability.TracerFromContext(ctx).Start(ctx, "ldap.find")
	defer observability.SpanEnd(span, err)

	span.SetAttributes(attribute.String("username", username))

	filter, err := c.usersFilter.Render(filterTemplateValues{
		Username: username,
	})
	if err != nil {
		return nil, fmt.Errorf("error while rendering user filter template: %w", err)
	}

	span.AddEvent("rendered find filter template string", trace.WithAttributes(attribute.String("filter", filter)))

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

	search, err := search(ctx, c.metrics, c.conn, searchRequest)
	if err != nil {
		return nil, fmt.Errorf("error while performing search for user: %w", err)
	}

	span.AddEvent("searched for machine users", trace.WithAttributes(attribute.Int("results", len(search.Entries))))

	if len(search.Entries) == 0 {
		return nil, ErrUserNotFound
	}

	if len(search.Entries) > 1 {
		return nil, ErrTooManyMatches
	}

	entry = search.Entries[0]

	cn := entry.GetAttributeValue("cn")
	if username != cn {
		return nil, fmt.Errorf("expected %q for username, but got %q: %w", username, cn, ErrNotMatchingUsername)
	}

	log := c.logger.With("username", username)
	for _, attr := range entry.Attributes {
		log = log.With(attr.Name, attr.Values)
	}

	log.DebugContext(c.ctx, "found user entry for username")

	return entry, nil
}
