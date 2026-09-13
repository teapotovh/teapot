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

func (c *Client) list(ctx context.Context, base, filter string) (entries []*ldap.Entry, err error) {
	ctx, span := observability.TracerFromContext(ctx).Start(ctx, "ldap.list")
	defer func() { observability.SpanEnd(span, err) }()

	searchRequest := ldap.NewSearchRequest(
		base,
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

	c.logger.DebugContext(c.ctx, "found entries", "base", base, "filter", filter, "dns", dns)

	return search.Entries, nil
}

func (c *Client) find(ctx context.Context, base, cn, filter string) (entry *ldap.Entry, err error) {
	ctx, span := observability.TracerFromContext(ctx).Start(ctx, "ldap.find")
	defer func() { observability.SpanEnd(span, err) }()

	searchRequest := ldap.NewSearchRequest(
		base,
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

	span.AddEvent("searched for matching users", trace.WithAttributes(attribute.Int("results", len(search.Entries))))

	if len(search.Entries) == 0 {
		return nil, ErrEntityNotFound
	}

	if len(search.Entries) > 1 {
		return nil, ErrTooManyMatches
	}

	entry = search.Entries[0]

	actualCN := entry.GetAttributeValue("cn")
	if cn != actualCN {
		return nil, fmt.Errorf("expected %q for CN, but got %q: %w", cn, actualCN, ErrNotMatchingUsername)
	}

	log := c.logger.With("cn", cn, "filter", filter)
	for _, attr := range entry.Attributes {
		log = log.With(attr.Name, attr.Values)
	}

	c.logger.DebugContext(c.ctx, "found entry for filter", "base", base, "filter", filter, "dn", entry.DN)

	return entry, nil
}
