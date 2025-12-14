package bottin

import (
	"fmt"
	"path"
	"strings"
	"unsafe"

	"github.com/teapotovh/teapot/service/bottin/store"
)

type ACL []ACLEntry

type ACLEntry struct {
	// The authenticated User (or ANONYMOUS if not authenticated) must match this string
	User string
	// For each of this groups, the authenticated user must belong to one group that matches
	RequiredGroups []string
	// The action requested must match one of these strings
	Actions []string
	// The requested Target must match this string. The special word SELF is replaced in the pattern by the user's dn
	// before matching
	Target string
	// All Attributes requested must match one of these patterns
	Attributes []store.AttributeKey
	// All attributes requested must not match any of these patterns
	ExcludedAttributes []store.AttributeKey
}

func splitNoEmpty(s string) []string {
	tmp := strings.Split(s, " ")
	ret := []string{}
	for _, s := range tmp {
		if len(s) > 0 {
			ret = append(ret, s)
		}
	}
	return ret
}

func parseACL(def []string) (ACL, error) {
	acl := []ACLEntry{}
	for _, item := range def {
		parts := strings.Split(item, ":")
		if len(parts) != 5 {
			return nil, fmt.Errorf("invalid ACL entry: %s", item)
		}
		var (
			attr     []store.AttributeKey
			exclAttr []store.AttributeKey
		)
		for _, s := range splitNoEmpty(parts[4]) {
			if s[0] == '!' {
				exclAttr = append(exclAttr, store.NewAttributeKey(s[1:]))
			} else {
				attr = append(attr, store.NewAttributeKey(s))
			}
		}
		entry := ACLEntry{
			User:               parts[0],
			RequiredGroups:     splitNoEmpty(parts[1]),
			Actions:            splitNoEmpty(parts[2]),
			Target:             parts[3],
			Attributes:         attr,
			ExcludedAttributes: exclAttr,
		}
		acl = append(acl, entry)
	}
	return acl, nil
}

type User struct {
	user   string
	groups []string
}

func (acl ACL) Check(login User, action string, target store.DN, attributes []store.AttributeKey) bool {
	tgt := target.String()
	for _, item := range acl {
		if item.Check(login, action, tgt, attributes) {
			return true
		}
	}
	return false
}

func attrstostrs(attrs []store.AttributeKey) []string {
	return *(*[]string)(unsafe.Pointer(&attrs))
}

func (entry *ACLEntry) Check(login User, action string, target string, attributes []store.AttributeKey) bool {
	if !match(entry.User, login.user) {
		return false
	}

	for _, grp := range entry.RequiredGroups {
		if !matchAny(grp, login.groups) {
			return false
		}
	}

	matchTarget := match(entry.Target, target)
	if !matchTarget && len(target) >= len(login.user) {
		start := len(target) - len(login.user)
		if target[start:] == login.user {
			matchTarget = match(entry.Target, target[:start]+"SELF")
		}
	}
	if !matchTarget {
		return false
	}

	if !anyMatch(entry.Actions, action) {
		return false
	}

	for _, attrib := range attributes {
		if !anyMatch(attrstostrs(entry.Attributes), string(attrib)) {
			return false
		}
	}

	for _, exclAttr := range entry.ExcludedAttributes {
		if matchAny(string(exclAttr), attrstostrs(attributes)) {
			return false
		}
	}

	return true
}

func match(pattern string, val string) bool {
	rv, err := path.Match(strings.ToLower(pattern), strings.ToLower(val))
	return err == nil && rv
}

func matchAny(pattern string, vals []string) bool {
	for _, val := range vals {
		if match(pattern, val) {
			return true
		}
	}
	return false
}

func anyMatch(patterns []string, val string) bool {
	for _, pattern := range patterns {
		if match(string(pattern), string(val)) {
			return true
		}
	}
	return false
}
