package bottin

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/google/uuid"

	"github.com/teapotovh/teapot/lib/ldapsrv"
	ldap "github.com/teapotovh/teapot/lib/ldapsrv/goldap"
	"github.com/teapotovh/teapot/service/bottin/store"
)

var (
	ErrHasChildren   = errors.New("has children")
	ErrNotFound      = errors.New("not found")
	ErrAlreadyExists = errors.New("already exists")
)

func (server *Bottin) Add(ctx context.Context, r ldap.AddRequest) error {
	user := ldapsrv.GetUser[User](ctx, EmptyUser)
	dn, err := server.parseDN(string(r.Entry()), false)
	if err != nil {
		return fmt.Errorf("(%w) %w", ldapsrv.ErrInvalidDNSyntax, err)
	}

	// Check permissions
	attrList := []store.AttributeKey{}
	for _, attribute := range r.Attributes() {
		attrList = append(attrList, store.NewAttributeKey(string(attribute.Type_())))
	}
	if !server.acl.Check(user, "add", dn, attrList) {
		return fmt.Errorf(
			"could not add %q: %w",
			dn,
			ldapsrv.ErrInsufficientAccessRights,
		)
	}

	server.logger.InfoContext(ctx, "adding entry", "dn", dn, "attributes", attrList)

	// Check that object does not already exist
	exists, err := server.existsEntry(ctx, dn)
	if err != nil {
		return fmt.Errorf("(%w) %w", ldapsrv.ErrOperationsError, err)
	}
	if exists {
		return fmt.Errorf("(%w) %w", ldapsrv.ErrEntryAlreadyExists, ErrAlreadyExists)
	}

	// Check that parent object exists
	parentDN := dn.Parent()
	parentExists, err := server.existsEntry(ctx, parentDN)
	if err != nil {
		return fmt.Errorf("(%w) %w", ldapsrv.ErrOperationsError, err)
	}
	if !parentExists {
		return fmt.Errorf(
			"(%w) parent object with DN %q does not exist",
			ldapsrv.ErrNoSuchObject,
			parentDN)
	}

	// If adding a group, track of who the members will be so that their memberOf field can be updated later
	var members []store.DN

	// Check attributes
	attrs := make(store.Attributes)
	for _, attribute := range r.Attributes() {
		key := store.NewAttributeKey(string(attribute.Type_()))
		vals := []string{}
		for _, val := range attribute.Vals() {
			vals = append(vals, string(val))
		}

		// Fail if they are trying to write memberOf, we manage this ourselves
		err = canUpdateAttribute(key)
		if err != nil {
			return fmt.Errorf("(%w) %w", ldapsrv.ErrObjectClassViolation, err)
		}
		if key.EqualFold(AttrMember) {
			// If they are writing a member list, we have to check they are adding valid members
			// Also, rewrite member list to use canonical DN syntax (no spaces, all lowercase)
			for _, member := range vals {
				memberCanonical, err := server.parseDN(member, false)
				if err != nil {
					return fmt.Errorf("(%w) %w", ldapsrv.ErrInvalidDNSyntax, err)
				}
				exists, err = server.existsEntry(ctx, memberCanonical)
				if err != nil {
					return fmt.Errorf("(%w) %w", ldapsrv.ErrOperationsError, err)
				}
				if !exists {
					return fmt.Errorf(
						"(%w) cannot add %q to members, it does not exist",
						ldapsrv.ErrNoSuchObject,
						memberCanonical)
				}
				members = append(members, memberCanonical)
			}

			var memberVal store.AttributeValue
			for _, member := range members {
				memberVal = append(memberVal, member.String())
			}
			attrs[key] = memberVal
		} else {
			attrs[key] = append(attrs[key], vals...)
		}
	}

	if len(attrs.Get(AttrObjectClass)) == 0 {
		attrs[AttrObjectClass] = store.AttributeValue{"top"}
	}

	uuid, err := uuid.NewRandom()
	if err != nil {
		return fmt.Errorf("(%w) error while generating random uuid: %w", ldapsrv.ErrOperationsError, err)
	}

	// Write system attributes
	attrs[AttrCreatorsName] = []string{user.user}
	attrs[AttrCreateTimestamp] = []string{genTimestamp()}
	attrs[AttrEntryUUID] = []string{uuid.String()}

	tx, err := server.store.Begin(ctx)
	if err != nil {
		return fmt.Errorf("(%w) error while beginning transaction: %w", ldapsrv.ErrOperationsError, err)
	}

	// This ensures the dn[].Type attribute is set to the appropriate value
	entry := store.NewEntry(dn, attrs)
	if err = tx.Store(entry); err != nil {
		return fmt.Errorf("(%w) error while storing entry: %w", ldapsrv.ErrOperationsError, err)
	}

	// If our item has a member list, add it to all of its member's memberOf attribute
	for _, member := range members {
		if err := server.membershipAdd(tx, AttrMemberOf, member, dn); err != nil {
			return fmt.Errorf(
				"(%w) error while adding %q to group %q: %w",
				ldapsrv.ErrOperationsError,
				member,
				dn,
				err,
			)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("(%w) could not commit transaction: %w", ldapsrv.ErrOperationsError, err)
	}

	return nil
}

func (server *Bottin) Del(ctx context.Context, r ldap.DelRequest) error {
	user := ldapsrv.GetUser[User](ctx, EmptyUser)

	dn, err := server.parseDN(string(r), false)
	if err != nil {
		return fmt.Errorf("(%w) %w", ldapsrv.ErrInvalidDNSyntax, err)
	}

	// Check for delete permission
	if !server.acl.Check(user, "delete", dn, []store.AttributeKey{}) {
		return fmt.Errorf(
			"could not delete %q: %w",
			dn,
			ldapsrv.ErrInsufficientAccessRights,
		)
	}

	server.logger.InfoContext(ctx, "deleting entry", "dn", dn)

	// Check that this LDAP entry exists and has no children
	entries, err := server.store.List(ctx, dn.Prefix(), false)
	if err != nil {
		return fmt.Errorf("(%w) error while fetching entry with DN %q from store: %w", ldapsrv.ErrOperationsError, dn.String(), err)
	}

	if len(entries) == 0 {
		return fmt.Errorf("(%w) error fetching entry %q: %w", ldapsrv.ErrNoSuchObject, ErrNotFound)
	}

	for _, entry := range entries {
		if !entry.DN.Equal(dn) {
			return fmt.Errorf(
				"(%w) cannot delete %q: %w", ldapsrv.ErrNotAllowedOnNonLeaf, dn, ErrHasChildren)
		}
	}

	entry := entries[0]

	// Retrieve group membership before we delete everything
	memberOf := entry.Get(AttrMemberOf)
	memberList := entry.Get(AttrMember)

	tx, err := server.store.Begin(ctx)
	if err != nil {
		return fmt.Errorf("(%w) error while beginning transaction: %w", ldapsrv.ErrOperationsError, err)
	}

	// Delete the LDAP entry
	if err = tx.Delete(dn); err != nil {
		return fmt.Errorf("(%w) error while deleting entry: %w", ldapsrv.ErrOperationsError, err)
	}

	// Delete it from the member list of all the groups it was a member of
	for _, group := range memberOf {
		gdn, err := server.parseDN(group, false)
		if err != nil {
			return fmt.Errorf("(%w) error while parsing DN from group members attribute: %w", ldapsrv.ErrInvalidDNSyntax, err)
		}

		err = server.membershipRemove(tx, AttrMember, gdn, dn)
		if err != nil {
			return fmt.Errorf("(%w) could not update attribute after removal: %w", ldapsrv.ErrOperationsError, err)
		}
	}

	// Delete it from all of its member's memberOf info
	for _, member := range memberList {
		mdn, err := server.parseDN(member, false)
		if err != nil {
			return fmt.Errorf("(%w) error while parsing DN from memberOf attribute: %w", ldapsrv.ErrInvalidDNSyntax, err)
		}

		if err := server.membershipRemove(tx, AttrMemberOf, mdn, dn); err != nil {
			return fmt.Errorf("(%w) error while removing memberOf: %w", ldapsrv.ErrOperationsError, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("(%w) could not commit transaction: %w", ldapsrv.ErrOperationsError, err)
	}

	return nil
}

func (server *Bottin) Modify(ctx context.Context, r ldap.ModifyRequest) error {
	user := ldapsrv.GetUser[User](ctx, EmptyUser)
	dn, err := server.parseDN(string(r.Object()), false)
	if err != nil {
		return fmt.Errorf("(%w) %w", ldapsrv.ErrInvalidDNSyntax, err)
	}

	// First permission check with no particular attributes
	if !server.acl.Check(user, "modify", dn, []store.AttributeKey{}) {
		return fmt.Errorf(
			"cannot not modify %q: %w",
			dn,
			ldapsrv.ErrInsufficientAccessRights,
		)
	}

	server.logger.InfoContext(ctx, "modifying entry", "dn", dn)

	prevEntry, err := server.getEntry(ctx, dn)
	if err != nil {
		return err
	}
	dnFirstComponent := prevEntry.DN[0]

	var (
		addMembers []store.DN
		delMembers []store.DN
	)

	// Produce new entry values to be saved
	attrs := make(store.Attributes)
	for _, change := range r.Changes() {
		attr := store.NewAttributeKey(string(change.Modification().Type_()))
		changeValues := []string{}
		for _, v := range change.Modification().Vals() {
			changeValues = append(changeValues, string(v))
		}

		// If we already had an attribute with this name before,
		// make sure we are using the same lowercase/uppercase
		for prevAttr := range prevEntry.Attributes {
			if attr.EqualFold(prevAttr) {
				attr = prevAttr
				break
			}
		}

		// Check that this attribute is not system-managed thus restricted
		err = canUpdateAttribute(attr)
		if err != nil {
			return fmt.Errorf("(%w) %w", ldapsrv.ErrObjectClassViolation, err)
		}

		if attr.EqualFold(store.NewAttributeKey(dnFirstComponent.Type)) {
			return fmt.Errorf(
				"(%w) %q may not be changed as it is part of object path",
				ldapsrv.ErrObjectClassViolation,
				attr,
			)
		}

		// Check for permission to modify this attribute
		if !server.acl.Check(user, "modify", dn, []store.AttributeKey{attr}) {
			return fmt.Errorf(
				"cannot not modify attribute %q on %q: %w",
				attr,
				dn,
				ldapsrv.ErrInsufficientAccessRights,
			)
		}

		// If we are changing ATTR_MEMBER, rewrite all values to canonical form
		if attr.EqualFold(AttrMember) {
			for i := range changeValues {
				canonicalVal, err := server.parseDN(changeValues[i], false)
				if err != nil {
					return fmt.Errorf("(%w) %w", ldapsrv.ErrInvalidDNSyntax, err)
				}
				changeValues[i] = canonicalVal.String()
			}
		}

		// If we don't yet have a new value for this attr,
		// but one existed before, initialize entry[attr] to the old value
		// so that later on what we do is simply modify entry[attr] in place
		// (this allows to handle sequences of several changes on the same attr)
		if _, ok := attrs[attr]; !ok {
			if _, ok := prevEntry.Attributes[attr]; ok {
				attrs[attr] = prevEntry.Attributes[attr]
			}
		}

		// Apply effective modification on entry[attr]
		if change.Operation() == ldapsrv.ModifyRequestChangeOperationAdd {
			for _, val := range changeValues {
				if !slices.Contains(attrs[attr], val) {
					attrs[attr] = append(attrs[attr], val)
					if attr.EqualFold(AttrMember) {
						valDN, err := server.parseDN(val, false)
						if err != nil {
							return fmt.Errorf("(%w) %w", ldapsrv.ErrInvalidDNSyntax, err)
						}
						addMembers = append(addMembers, valDN)
					}
				}
			}
		} else if change.Operation() == ldapsrv.ModifyRequestChangeOperationDelete {
			if len(changeValues) == 0 {
				// Delete everything
				if attr.EqualFold(AttrMember) {
					for _, val := range attrs[attr] {
						valDN, err := server.parseDN(val, false)
						if err != nil {
							return fmt.Errorf("(%w) %w", ldapsrv.ErrInvalidDNSyntax, err)
						}
						delMembers = append(delMembers, valDN)
					}
				}

				attrs[attr] = store.AttributeValue{}
			} else {
				// Delete only those specified
				newList := []string{}
				for _, prevVal := range attrs[attr] {
					if !slices.Contains(changeValues, prevVal) {
						newList = append(newList, prevVal)
					} else {
						if attr.EqualFold(AttrMember) {
							valDN, err := server.parseDN(prevVal, false)
							if err != nil {
								return fmt.Errorf("(%w) %w", ldapsrv.ErrInvalidDNSyntax, err)
							}
							delMembers = append(delMembers, valDN)
						}
					}
				}
				attrs[attr] = newList
			}
		} else if change.Operation() == ldapsrv.ModifyRequestChangeOperationReplace {
			if attr.EqualFold(AttrMember) {
				for _, newMem := range changeValues {
					if !slices.Contains(attrs[attr], newMem) {
						valDN, err := server.parseDN(newMem, false)
						if err != nil {
							return fmt.Errorf("(%w) %w", ldapsrv.ErrInvalidDNSyntax, err)
						}
						addMembers = append(addMembers, valDN)
					}
				}
				for _, prevMem := range attrs[attr] {
					if !slices.Contains(changeValues, prevMem) {
						valDN, err := server.parseDN(prevMem, false)
						if err != nil {
							return fmt.Errorf("(%w) %w", ldapsrv.ErrInvalidDNSyntax, err)
						}
						delMembers = append(delMembers, valDN)
					}
				}
			}
			attrs[attr] = changeValues
		}
	}

	// Check that added members actually exist
	for i := range addMembers {
		exists, err := server.existsEntry(ctx, addMembers[i])
		if err != nil {
			return fmt.Errorf("(%w) %w", ldapsrv.ErrOperationsError, err)
		}
		if !exists {
			return fmt.Errorf("(%w) cannot add member %q, it does not exist", ldapsrv.ErrNoSuchObject, addMembers[i])
		}
	}

	for k, v := range attrs {
		if k.EqualFold(AttrObjectClass) && len(v) == 0 {
			return fmt.Errorf(
				"(%w) cannot remove all objectclass values", ldapsrv.ErrInsufficientAccessRights)
		}
	}

	// Now, the modification has been processed and accepted and we want to commit it
	attrs[AttrModifiersName] = []string{user.user}
	attrs[AttrModifyTimestamp] = []string{genTimestamp()}

	tx, err := server.store.Begin(ctx)
	if err != nil {
		return fmt.Errorf("(%w) error while beginning transaction: %w", ldapsrv.ErrOperationsError, err)
	}

	// Save the edited values
	entry := store.NewEntry(prevEntry.DN, attrs)
	if err = tx.Store(entry); err != nil {
		return fmt.Errorf("(%w) error while storing updated entry: %w", ldapsrv.ErrOperationsError, err)
	}

	// Update memberOf for added members and deleted members
	for _, addMem := range addMembers {
		if err := server.membershipAdd(tx, AttrMemberOf, addMem, dn); err != nil {
			return fmt.Errorf("(%w) error while adding memberOf: %w", ldapsrv.ErrOperationsError, err)
		}
	}

	for _, delMem := range delMembers {
		if err := server.membershipRemove(tx, AttrMemberOf, delMem, dn); err != nil {
			return fmt.Errorf("(%w) error while removing memberOf: %w", ldapsrv.ErrOperationsError, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("(%w) could not commit transaction: %w", ldapsrv.ErrOperationsError, err)
	}

	return nil
}
