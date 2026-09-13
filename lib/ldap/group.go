package ldap

import (
	"context"
	"fmt"
	"strconv"

	"github.com/go-ldap/ldap/v3"
	"github.com/teapotovh/teapot/lib/observability"
	"go.opentelemetry.io/otel/attribute"
)

// Group is an abstracted view over a group entry in LDAP.
type Group struct {
	DN        string
	Groupname string
	GID       int
	Memebers  []string
}

func (c *Client) findGroup(ctx context.Context, groupname string) (entry *ldap.Entry, err error) {
	filter, err := c.groupsFilter.Render(groupFilterTemplateValues{
		Groupname: groupname,
	})
	if err != nil {
		return nil, fmt.Errorf("error while rendering group filter template: %w", err)
	}

	return c.find(ctx, c.groupsDN, groupname, filter)
}

func (c *Client) mapGroup(entry *ldap.Entry) (*Group, error) {
	dn := entry.DN
	groupname := entry.GetAttributeValue("cn")
	rawUID := entry.GetAttributeValue("gidnumber")

	gid, err := strconv.Atoi(rawUID)
	if err != nil {
		return nil, fmt.Errorf("error while parsing gid: %w", err)
	}

	rawMemebers := entry.GetAttributeValues("member")
	members := filterDN(rawMemebers, c.usersDN)

	return &Group{
		DN:        dn,
		Groupname: groupname,
		GID:       gid,
		Memebers:  members,
	}, nil
}

func (c *Client) Group(ctx context.Context, groupname string) (group *Group, err error) {
	ctx, span := observability.TracerFromContext(ctx).Start(ctx, "Client.Group")
	defer func() { observability.SpanEnd(span, err) }()

	span.SetAttributes(attribute.String("groupname", groupname))

	defer func() {
		if err != nil {
			c.errored = true
		}
	}()

	entry, err := c.findGroup(ctx, groupname)
	if err != nil {
		return nil, fmt.Errorf("error while looking up group: %w", err)
	}

	return c.mapGroup(entry)
}

func (c *Client) Groups(ctx context.Context) (groups []*Group, err error) {
	ctx, span := observability.TracerFromContext(ctx).Start(ctx, "Client.Groups")
	defer func() { observability.SpanEnd(span, err) }()

	defer func() {
		if err != nil {
			c.errored = true
		}
	}()

	filter, err := c.groupsFilter.Render(groupFilterTemplateValues{Groupname: "*"})
	if err != nil {
		return nil, fmt.Errorf("error while rendering group filter template: %w", err)
	}

	entries, err := c.list(ctx, c.groupsDN, filter)
	if err != nil {
		return nil, fmt.Errorf("error while listing all groups: %w", err)
	}

	for _, entry := range entries {
		group, err := c.mapGroup(entry)
		if err != nil {
			groupname := entry.GetAttributeValue("cn")
			return nil, fmt.Errorf("error while mapping group %q: %w", groupname, err)
		}

		groups = append(groups, group)
	}

	return groups, nil
}
