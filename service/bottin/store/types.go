package store

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
)

const (
	dnSeparator     = ","
	prefixSeparator = "/"
)

var ErrMissingEquals = errors.New("missing = in DN, expected exactly one")

func reverse[T any](slice []T) []T {
	cpy := slices.Clone(slice)
	slices.Reverse(cpy)
	return cpy
}

type Component struct {
	Type  string
	Value string
}

func NewComponent(typ, value string) Component {
	return Component{Type: typ, Value: value}
}

func (seg Component) String() string {
	return fmt.Sprintf("%s=%s", seg.Type, seg.Value)
}

func ParseComponent(rawComp string) (Component, error) {
	splits := strings.Split(rawComp, "=")
	if len(splits) != 2 {
		err := fmt.Errorf("invalid DN component: %s", rawComp)
		return Component{}, errors.Join(err, ErrMissingEquals)
	}

	return Component{
		Type:  strings.ToLower(strings.TrimSpace(splits[0])),
		Value: strings.ToLower(strings.TrimSpace(splits[1])),
	}, nil
}

func parseComponentSlice(raw, separator string) ([]Component, error) {
	strs := strings.Split(raw, separator)
	var components []Component

	for _, str := range strs {
		component, err := ParseComponent(str)
		if err != nil {
			return nil, err
		}

		components = append(components, component)
	}

	return components, nil
}

func joinComponentSlice(comps []Component, separator string) string {
	var rawComps []string
	for _, comp := range comps {
		rawComps = append(rawComps, comp.String())
	}

	return strings.Join(rawComps, separator)
}

// DN represents an LDAP DN split by ,.
type DN []Component

func (dn DN) Prefix() Prefix {
	return Prefix(reverse(dn))
}

func (dn DN) String() string {
	return joinComponentSlice(dn, dnSeparator)
}

func (dn DN) Clone() DN {
	return slices.Clone(dn)
}

func (dn DN) Level() int {
	return len(dn)
}

func (dn DN) Equal(dn2 DN) bool {
	return slices.Equal(dn, dn2)
}

func (dn DN) Sub(comps ...Component) DN {
	return append(comps, dn.Clone()...)
}

func (dn DN) Parent() DN {
	return dn.Clone()[1:]
}

func ParseDN(rawDN string) (DN, error) {
	return parseComponentSlice(rawDN, dnSeparator)
}

// Prefix is the reverse of a DN and can be used to recursively enumerate the directory.
// If you take two DNs (of length n and m respectively) and get their prefixes,
// you can check if the two DNs are one subtree of the other by checking if the
// fist min(n, m) parts match.
// For example:
// DN1: dc=teapot,dc=ovh           PR1: dc=ovh/dc=teapot
// DN2: ou=users,dc=teapot,dc=ovh  PR2: dc=ovh/dc=teapot/ou=users
// Checking if strings.HasPrefix(PR1, PR2) will tell you if one is subtree of the other.
type Prefix []Component

func (prefix Prefix) DN() DN {
	return DN(reverse(prefix))
}

func (prefix Prefix) String() string {
	return joinComponentSlice(prefix, prefixSeparator)
}

func (prefix Prefix) Clone() Prefix {
	return slices.Clone(prefix)
}

func (prefix Prefix) Level() int {
	return len(prefix)
}

func (prefix Prefix) Equal(prefix2 Prefix) bool {
	return slices.Equal(prefix, prefix2)
}

func (prefix Prefix) IsPrefixOf(prf Prefix) bool {
	if len(prf) < len(prefix) {
		return false
	}

	return slices.Equal(prefix, prf[:len(prefix)])
}

func ParsePrefix(rawPrefix string) (Prefix, error) {
	return parseComponentSlice(rawPrefix, prefixSeparator)
}

type (
	AttributeKey   string
	AttributeValue []string
	Attributes     map[AttributeKey]AttributeValue
)

func NewAttributeKey(key string) AttributeKey {
	return AttributeKey(strings.ToLower(key))
}

func (ak1 AttributeKey) EqualFold(ak2 AttributeKey) bool {
	return strings.EqualFold(string(ak1), string(ak2))
}

type Entry struct {
	Attributes Attributes
	DN         DN
}

func NewEntry(dn DN, attributes Attributes) Entry {
	attrs := maps.Clone(attributes)
	if len(dn) > 0 {
		dnFirstComponent := dn[0]
		attrs[NewAttributeKey(dnFirstComponent.Type)] = AttributeValue{dnFirstComponent.Value}
	}

	return Entry{DN: dn, Attributes: attrs}
}

func (attrs Attributes) Get(key AttributeKey) AttributeValue {
	value, ok := attrs[key]
	if !ok {
		return AttributeValue{}
	}

	return value
}

func (entry *Entry) Get(key AttributeKey) AttributeValue {
	return entry.Attributes.Get(key)
}
