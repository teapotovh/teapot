package bottin

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/teapotovh/teapot/lib/ldapsrv"
	ldap "github.com/teapotovh/teapot/lib/ldapsrv/goldap"
	"github.com/teapotovh/teapot/service/bottin/store"
)

var (
	ErrUnsupportedFilter = errors.New("unsupported filter")
)

func (server *Bottin) getEntry(ctx context.Context, dn store.DN) (*store.Entry, error) {
	entries, err := server.store.List(ctx, dn.Prefix(), true)
	if err != nil {
		return nil, fmt.Errorf("(%w) error while fetching entry with DN %q from store: %w", ldapsrv.ErrOperationsError, dn.String(), err)
	}

	if len(entries) < 1 {
		return nil, fmt.Errorf("(%w) error while fetching entry %q: %w", ldapsrv.ErrNoSuchObject, dn.String(), ErrNotFound)
	}

	if len(entries) > 1 {
		return nil, fmt.Errorf("(%w) error while fetching entry %q: %w", ldapsrv.ErrOperationsError, dn.String(), ErrNotFound)
	}

	return &entries[0], nil
}

func (server *Bottin) existsEntry(ctx context.Context, dn store.DN) (bool, error) {
	entries, err := server.store.List(ctx, dn.Prefix(), true)
	if err != nil {
		return false, fmt.Errorf("error while checking if entry with DN %q exists: %w", dn.String(), err)
	}

	return len(entries) == 1, nil
}

func (server *Bottin) Compare(ctx context.Context, r ldap.CompareRequest) (bool, error) {
	user := ldapsrv.GetUser[User](ctx, EmptyUser)
	attr := store.NewAttributeKey(string(r.Ava().AttributeDesc()))
	expected := string(r.Ava().AssertionValue())

	dn, err := server.parseDN(string(r.Entry()), false)
	if err != nil {
		return false, fmt.Errorf("(%w) %w", ldapsrv.ErrInvalidDNSyntax, err)
	}

	// Check permissions
	if !server.acl.Check(user, "read", dn, []store.AttributeKey{attr}) {
		return false, fmt.Errorf(
			"could not read %q: %w",
			dn,
			ldapsrv.ErrInsufficientAccessRights,
		)
	}

	server.logger.InfoContext(ctx, "comparing entry", "dn", dn, "attr", attr)

	entry, err := server.getEntry(ctx, dn)
	if err != nil {
		return false, err
	}

	values := entry.Get(attr)
	for _, v := range values {
		if valueMatch(attr, v, expected) {
			return true, nil
		}
	}

	return false, nil
}

//nolint:all
func (server *Bottin) Search(ctx context.Context, r ldap.SearchRequest) ([]ldap.SearchResultEntry, error) {
	user := ldapsrv.GetUser[User](ctx, EmptyUser)
	baseObject, err := server.parseDN(string(r.BaseObject()), true)
	if err != nil {
		return nil, fmt.Errorf("(%w) %w", ldapsrv.ErrInvalidDNSyntax, err)
	}

	server.logger.InfoContext(
		ctx,
		"searching for entries",
		"base",
		baseObject,
		"filter",
		r.FilterString(),
		"attributes",
		r.Attributes(),
		"deadline",
		r.TimeLimit(),
	)

	if !server.acl.Check(user, "read", baseObject, []store.AttributeKey{}) {
		return nil, fmt.Errorf(
			"could not read %q: %w",
			baseObject,
			ldapsrv.ErrInsufficientAccessRights,
		)
	}

	baseObjectLevel := baseObject.Level()
	exact := r.Scope() == ldap.SearchRequestScopeBaseObject
	entries, err := server.store.List(ctx, baseObject.Prefix(), exact)
	if err != nil {
		return nil, fmt.Errorf("(%w), error while listing objects: %w", ldapsrv.ErrOperationsError, err)
	}

	server.logger.DebugContext(ctx, "retrieved entries", "entries", entries, "base", baseObject)

	var results []ldap.SearchResultEntry
	for _, entry := range entries {
		if r.Scope() == ldap.SearchRequestScopeBaseObject {
			if entry.DN.Equal(baseObject) {
				continue
			}
		} else if r.Scope() == ldap.SearchRequestSingleLevel {
			if entry.DN.Level() != baseObjectLevel+1 {
				continue
			}
		}
		// Filter out if we don't match requested filter
		matched, err := applyFilter(entry, r.Filter())
		if err != nil {
			return nil, fmt.Errorf("(%w), error while applying filter %q on %q: %w", ldapsrv.ErrUnwillingToPerform, r.Filter(), entry.DN.String(), err)
		}
		if !matched {
			continue
		}

		// Filter out if user is not allowed to read this
		if !server.acl.Check(user, "read", entry.DN, []store.AttributeKey{}) {
			continue
		}

		e := ldapsrv.NewSearchResultEntry(entry.DN.String())
		for attr, val := range entry.Attributes {
			// If attribute is not in request, exclude it from returned entry
			if len(r.Attributes()) > 0 {
				found := false
				for _, requested := range r.Attributes() {
					if string(requested) == "1.1" && len(r.Attributes()) == 1 {
						found = false
						break
					}
					if (string(requested) == "*" && !isOperationalAttribute(attr)) ||
						(string(requested) == "+" && isOperationalAttribute(attr)) ||
						strings.EqualFold(string(requested), string(attr)) {
						found = true
						break
					}
				}
				if !found {
					continue
				}
			}

			// If we are not allowed to read attribute, exclude it from returned entry
			if !server.acl.Check(user, "read", entry.DN, []store.AttributeKey{attr}) {
				continue
			}

			// Send result
			resultVals := []ldap.AttributeValue{}
			for _, v := range val {
				resultVals = append(resultVals, ldap.AttributeValue(v))
			}
			e.AddAttribute(ldap.AttributeDescription(attr), resultVals...)
		}

		results = append(results, e)
	}

	return results, nil
}

//nolint:gocyclo
func applyFilter(entry store.Entry, filter ldap.Filter) (bool, error) {
	if fAnd, ok := filter.(ldap.FilterAnd); ok {
		for _, cond := range fAnd {
			res, err := applyFilter(entry, cond)
			if err != nil {
				return false, err
			}

			if !res {
				return false, nil
			}
		}

		return true, nil
	} else if fOr, ok := filter.(ldap.FilterOr); ok {
		for _, cond := range fOr {
			res, err := applyFilter(entry, cond)
			if err != nil {
				return false, err
			}

			if res {
				return true, nil
			}
		}

		return false, nil
	} else if fNot, ok := filter.(ldap.FilterNot); ok {
		res, err := applyFilter(entry, fNot.Filter)
		if err != nil {
			return false, err
		}

		return !res, nil
	} else if fPresent, ok := filter.(ldap.FilterPresent); ok {
		what := string(fPresent)
		// Case insensitive search
		for desc, values := range entry.Attributes {
			if strings.EqualFold(what, string(desc)) {
				return len(values) > 0, nil
			}
		}

		return false, nil
	} else if fEquality, ok := filter.(ldap.FilterEqualityMatch); ok {
		desc := string(fEquality.AttributeDesc())
		target := string(fEquality.AssertionValue())
		// Case insensitive attribute search
		for entryDesc, value := range entry.Attributes {
			if strings.EqualFold(string(entryDesc), desc) {
				for _, val := range value {
					if valueMatch(entryDesc, val, target) {
						return true, nil
					}
				}

				return false, nil
			}
		}

		return false, nil
	} else {
		return false, fmt.Errorf("error while applying filter %#v (of type %T): %w", filter, filter, ErrUnsupportedFilter)
	}
}
