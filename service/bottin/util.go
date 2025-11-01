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
		ATTR_ENTRYUUID,
		ATTR_CREATORSNAME,
		ATTR_CREATETIMESTAMP,
		ATTR_MODIFIERSNAME,
		ATTR_MODIFYTIMESTAMP,
		"entrycsn",
	}

	ErrMememberOfDefinition = errors.New("memberOf cannot be defined directly, membership must be specified in the group itself")
	ErrRestrictedAttribute  = errors.New("Attribute is restricted and may only be set by the system")
)

func isOperationalAttribute(attr store.AttributeKey) bool {
	return slices.ContainsFunc(OperationalAttributes, attr.EqualFold)
}

func canUpdateAttribute(attr store.AttributeKey) error {
	if attr.EqualFold(ATTR_MEMBEROF) {
		return ErrMememberOfDefinition
	}

	if isOperationalAttribute(attr) {
		return fmt.Errorf("Cannot modify attribute %s: %w", attr, ErrRestrictedAttribute)
	}

	return nil
}

func genTimestamp() string {
	return time.Now().Format("20060102150405Z")
}

func valueMatch(attr store.AttributeKey, val1, val2 string) bool {
	if attr.EqualFold(ATTR_USERPASSWORD) {
		return val1 == val2
	} else {
		return strings.EqualFold(val1, val2)
	}
}
