package bottin

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/teapotovh/teapot/service/bottin/store"
)

var (
	OperationalAttributes = []store.AttributeKey{
		AttrEntryUUID,
		AttrCreatorsName,
		AttrCreateTimestamp,
		AttrModifiersName,
		AttrModifyTimestamp,
		"entrycsn",
	}

	ErrMememberOfDefinition = errors.New(
		"memberOf cannot be defined directly, membership must be specified in the group itself",
	)
	ErrRestrictedAttribute = errors.New("attribute is restricted and may only be set by the system")
)

func isOperationalAttribute(attr store.AttributeKey) bool {
	return slices.ContainsFunc(OperationalAttributes, attr.EqualFold)
}

func canUpdateAttribute(attr store.AttributeKey) error {
	if attr.EqualFold(AttrMemberOf) {
		return ErrMememberOfDefinition
	}

	if isOperationalAttribute(attr) {
		return fmt.Errorf("cannot modify attribute %q: %w", attr, ErrRestrictedAttribute)
	}

	return nil
}

func genTimestamp() string {
	return time.Now().Format("20060102150405Z")
}

func valueMatch(attr store.AttributeKey, val1, val2 string) bool {
	if attr.EqualFold(AttrUserPassword) {
		return val1 == val2
	} else {
		return strings.EqualFold(val1, val2)
	}
}
