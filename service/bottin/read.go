package bottin

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/teapotovh/teapot/lib/ldapserver"
	goldap "github.com/teapotovh/teapot/lib/ldapserver/goldap"
	"github.com/teapotovh/teapot/service/bottin/store"
)

var (
	ErrUnsupportedFilter = errors.New("unsupported filter")
)

// TODO: return ResultCodeNoSuchObject if len(entries) < 1
// TODO: return ResultCodeOperationsError if len(entries) > 1.
func (server *Bottin) getEntry(ctx context.Context, dn store.DN) (*store.Entry, error) {
	entries, err := server.store.List(ctx, dn.Prefix(), true)
	if err != nil {
		return nil, fmt.Errorf("error while fetching entry with DN %q from store: %w", dn.String(), err)
	}

	if len(entries) != 1 {
		return nil, fmt.Errorf("error while fetching entry %q: %w", dn.String(), ErrNotFound)
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

func (server *Bottin) HandleCompare(
	ctx context.Context,
	w ldapserver.ResponseWriter,
	m *ldapserver.Message,
) context.Context {
	r := m.GetCompareRequest()

	code, err := server.handleCompareInternal(ctx, &r)

	res := ldapserver.NewResponse(code)
	if err != nil {
		res.SetDiagnosticMessage(err.Error())
	}

	w.Write(goldap.CompareResponse(res))

	return ctx
}

func (server *Bottin) handleCompareInternal(ctx context.Context, r *goldap.CompareRequest) (int32, error) {
	user := ldapserver.GetUser[User](ctx, EmptyUser)
	attr := store.NewAttributeKey(string(r.Ava().AttributeDesc()))
	expected := string(r.Ava().AssertionValue())

	dn, err := server.parseDN(string(r.Entry()), false)
	if err != nil {
		return goldap.ResultCodeInvalidDNSyntax, err
	}

	// Check permissions
	if !server.acl.Check(user, "read", dn, []store.AttributeKey{attr}) {
		return goldap.ResultCodeInsufficientAccessRights, nil
	}

	server.logger.InfoContext(ctx, "comparing entry", "dn", dn, "attr", attr)

	entry, err := server.getEntry(ctx, dn)
	if err != nil {
		return goldap.ResultCodeOperationsError, err
	}

	values := entry.Get(attr)
	for _, v := range values {
		if valueMatch(attr, v, expected) {
			return goldap.ResultCodeCompareTrue, nil
		}
	}

	return goldap.ResultCodeCompareFalse, nil
}

func (server *Bottin) HandleSearch(
	ctx context.Context,
	w ldapserver.ResponseWriter,
	m *ldapserver.Message,
) context.Context {
	r := m.GetSearchRequest()
	code, err := server.handleSearchInternal(ctx, w, &r)

	res := ldapserver.NewResponse(code)
	if err != nil {
		res.SetDiagnosticMessage(err.Error())
	}

	if code != goldap.ResultCodeSuccess {
		server.logger.ErrorContext(ctx, "error while performing search", "req", r, "err", err)
	}

	w.Write(goldap.SearchResultDone(res))

	return ctx
}

//nolint:all
func (server *Bottin) handleSearchInternal(
	ctx context.Context,
	w ldapserver.ResponseWriter,
	r *goldap.SearchRequest,
) (int32, error) {
	user := ldapserver.GetUser[User](ctx, EmptyUser)
	baseObject, err := server.parseDN(string(r.BaseObject()), true)
	if err != nil {
		return goldap.ResultCodeInvalidDNSyntax, err
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
		return goldap.ResultCodeInsufficientAccessRights, fmt.Errorf(
			"please specify a base object on which you have read rights",
		)
	}

	baseObjectLevel := baseObject.Level()
	exact := r.Scope() == goldap.SearchRequestScopeBaseObject
	entries, err := server.store.List(ctx, baseObject.Prefix(), exact)
	if err != nil {
		return goldap.ResultCodeOperationsError, err
	}

	server.logger.DebugContext(ctx, "retrieved entries", "entries", entries, "base", baseObject)

	for _, entry := range entries {
		if r.Scope() == goldap.SearchRequestScopeBaseObject {
			if entry.DN.Equal(baseObject) {
				continue
			}
		} else if r.Scope() == goldap.SearchRequestSingleLevel {
			if entry.DN.Level() != baseObjectLevel+1 {
				continue
			}
		}
		// Filter out if we don't match requested filter
		matched, err := applyFilter(entry, r.Filter())
		if err != nil {
			return goldap.ResultCodeUnwillingToPerform, err
		}
		if !matched {
			continue
		}

		// Filter out if user is not allowed to read this
		if !server.acl.Check(user, "read", entry.DN, []store.AttributeKey{}) {
			continue
		}

		e := ldapserver.NewSearchResultEntry(entry.DN.String())
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
			resultVals := []goldap.AttributeValue{}
			for _, v := range val {
				resultVals = append(resultVals, goldap.AttributeValue(v))
			}
			e.AddAttribute(goldap.AttributeDescription(attr), resultVals...)
		}
		w.Write(e)
	}

	return goldap.ResultCodeSuccess, nil
}

//nolint:gocyclo
func applyFilter(entry store.Entry, filter goldap.Filter) (bool, error) {
	if fAnd, ok := filter.(goldap.FilterAnd); ok {
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
	} else if fOr, ok := filter.(goldap.FilterOr); ok {
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
	} else if fNot, ok := filter.(goldap.FilterNot); ok {
		res, err := applyFilter(entry, fNot.Filter)
		if err != nil {
			return false, err
		}

		return !res, nil
	} else if fPresent, ok := filter.(goldap.FilterPresent); ok {
		what := string(fPresent)
		// Case insensitive search
		for desc, values := range entry.Attributes {
			if strings.EqualFold(what, string(desc)) {
				return len(values) > 0, nil
			}
		}

		return false, nil
	} else if fEquality, ok := filter.(goldap.FilterEqualityMatch); ok {
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
