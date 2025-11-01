package bottin

import (
	"context"
	"fmt"
	"slices"

	"github.com/google/uuid"
	"github.com/teapotovh/teapot/lib/ldapserver"
	goldap "github.com/teapotovh/teapot/lib/ldapserver/goldap"
	"github.com/teapotovh/teapot/service/bottin/store"
)

func (server *Bottin) HandleAdd(ctx context.Context, w ldapserver.ResponseWriter, m *ldapserver.Message) context.Context {
	r := m.GetAddRequest()

	code, err := server.handleAddInternal(ctx, &r)

	res := ldapserver.NewResponse(code)
	if err != nil {
		res.SetDiagnosticMessage(err.Error())
	}
	if code == goldap.ResultCodeSuccess {
		server.logger.InfoContext(ctx, "successfully added", "entry", r.Entry())
	} else {
		server.logger.ErrorContext(ctx, "error while adding entry", "entry", r.Entry(), "err", err)
	}
	w.Write(goldap.AddResponse(res))
	return ctx
}

func (server *Bottin) handleAddInternal(ctx context.Context, r *goldap.AddRequest) (int, error) {
	user := ldapserver.GetUser[User](ctx, EmptyUser)
	dn, err := server.parseDN(string(r.Entry()), false)
	if err != nil {
		return goldap.ResultCodeInvalidDNSyntax, err
	}

	// Check permissions
	attrList := []store.AttributeKey{}
	for _, attribute := range r.Attributes() {
		attrList = append(attrList, store.NewAttributeKey(string(attribute.Type_())))
	}
	if !server.acl.Check(user, "add", dn, attrList) {
		return goldap.ResultCodeInsufficientAccessRights, nil
	}

	server.logger.InfoContext(ctx, "adding entry", "dn", dn, "attributes", attrList)

	// Check that object does not already exist
	exists, err := server.existsEntry(ctx, dn)
	if err != nil {
		return goldap.ResultCodeOperationsError, err
	}
	if exists {
		return goldap.ResultCodeEntryAlreadyExists, nil
	}

	// Check that parent object exists
	parentDn := dn.Parent()
	parentExists, err := server.existsEntry(ctx, parentDn)
	if err != nil {
		return goldap.ResultCodeOperationsError, err
	}
	if !parentExists {
		return goldap.ResultCodeNoSuchObject, fmt.Errorf("parent object with DN %s does not exist", parentDn.String())
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
			return goldap.ResultCodeObjectClassViolation, err
		}
		if key.EqualFold(ATTR_MEMBER) {
			// If they are writing a member list, we have to check they are adding valid members
			// Also, rewrite member list to use canonical DN syntax (no spaces, all lowercase)
			for _, member := range vals {
				memberCanonical, err := server.parseDN(member, false)
				if err != nil {
					return goldap.ResultCodeInvalidDNSyntax, err
				}
				exists, err = server.existsEntry(ctx, memberCanonical)
				if err != nil {
					return goldap.ResultCodeOperationsError, err
				}
				if !exists {
					return goldap.ResultCodeNoSuchObject, fmt.Errorf(
						"Cannot add %s to members, it does not exist!",
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

	if len(attrs.Get(ATTR_OBJECTCLASS)) == 0 {
		attrs[ATTR_OBJECTCLASS] = store.AttributeValue{"top"}
	}

	uuid, err := uuid.NewRandom()
	if err != nil {
		return goldap.ResultCodeOperationsError, fmt.Errorf("error while generating random uuid: %w", err)
	}

	// Write system attributes
	attrs[ATTR_CREATORSNAME] = []string{user.user}
	attrs[ATTR_CREATETIMESTAMP] = []string{genTimestamp()}
	attrs[ATTR_ENTRYUUID] = []string{uuid.String()}

	tx, err := server.store.Begin(ctx)
	if err != nil {
		return goldap.ResultCodeOperationsError, fmt.Errorf("error while beginning transaction: %w", err)
	}

	// This ensures the dn[].Type attribute is set to the appropriate value
	entry := store.NewEntry(dn, attrs)
	if err = tx.Store(entry); err != nil {
		return goldap.ResultCodeOperationsError, fmt.Errorf("error while storing entry: %w", err)
	}

	// If our item has a member list, add it to all of its member's memberOf attribute
	for _, member := range members {
		if err := server.membershipAdd(tx, ATTR_MEMBEROF, member, dn); err != nil {
			return goldap.ResultCodeOperationsError, fmt.Errorf("error while adding %s to group %s: %w", member.String(), dn.String(), err)
		}
	}

	if err := tx.Commit(); err != nil {
		return goldap.ResultCodeOperationsError, fmt.Errorf("could not commit transaction: %w", err)
	}

	return goldap.ResultCodeSuccess, nil
}

// Delete request ------------------------

func (server *Bottin) HandleDelete(ctx context.Context, w ldapserver.ResponseWriter, m *ldapserver.Message) context.Context {
	r := m.GetDeleteRequest()

	code, err := server.handleDeleteInternal(ctx, &r)

	res := ldapserver.NewResponse(code)
	if err != nil {
		res.SetDiagnosticMessage(err.Error())
	}
	if code == goldap.ResultCodeSuccess {
		server.logger.InfoContext(ctx, "successfully deleted", "req", r)
	} else {
		server.logger.ErrorContext(ctx, "error while deleting entry", "req", r, "err", err)
	}
	w.Write(goldap.DelResponse(res))
	return ctx
}

func (server *Bottin) handleDeleteInternal(ctx context.Context, r *goldap.DelRequest) (int, error) {
	user := ldapserver.GetUser[User](ctx, EmptyUser)
	dn, err := server.parseDN(string(*r), false)
	if err != nil {
		return goldap.ResultCodeInvalidDNSyntax, err
	}

	// Check for delete permission
	if !server.acl.Check(user, "delete", dn, []store.AttributeKey{}) {
		return goldap.ResultCodeInsufficientAccessRights, nil
	}

	server.logger.InfoContext(ctx, "deleting entry", "dn", dn)

	// Check that this LDAP entry exists and has no children
	entries, err := server.store.List(ctx, dn.Prefix(), false)
	if err != nil {
		return goldap.ResultCodeOperationsError, err
	}

	if len(entries) == 0 {
		return goldap.ResultCodeNoSuchObject, fmt.Errorf("Not found: %s", dn)
	}
	for _, entry := range entries {
		if !entry.DN.Equal(dn) {
			return goldap.ResultCodeNotAllowedOnNonLeaf, fmt.Errorf(
				"Cannot delete %s as it has children", dn)
		}
	}
	entry := entries[0]

	// Retrieve group membership before we delete everything
	memberOf := entry.Get(ATTR_MEMBEROF)
	memberList := entry.Get(ATTR_MEMBER)

	tx, err := server.store.Begin(ctx)
	if err != nil {
		return goldap.ResultCodeOperationsError, fmt.Errorf("error while beginning transaction: %w", err)
	}

	// Delete the LDAP entry
	if err = tx.Delete(dn); err != nil {
		return goldap.ResultCodeOperationsError, err
	}

	// Delete it from the member list of all the groups it was a member of
	if memberOf != nil {
		for _, group := range memberOf {
			gdn, err := server.parseDN(group, false)
			if err != nil {
				return goldap.ResultCodeOperationsError, fmt.Errorf("error while parsing DN from group members attribute: %w", err)
			}

			err = server.membershipRemove(tx, ATTR_MEMBER, gdn, dn)
			if err != nil {
				return goldap.ResultCodeOperationsError, fmt.Errorf("could not update attribute after removal: %w", err)
			}
		}
	}

	// Delete it from all of its member's memberOf info
	for _, member := range memberList {
		mdn, err := server.parseDN(member, false)
		if err != nil {
			return goldap.ResultCodeOperationsError, fmt.Errorf("error while parsing DN from memberOf attribute: %w", err)
		}

		if err := server.membershipRemove(tx, ATTR_MEMBEROF, mdn, dn); err != nil {
			return goldap.ResultCodeOperationsError, fmt.Errorf("error while removing memberOf: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return goldap.ResultCodeOperationsError, fmt.Errorf("could not commit transaction: %w", err)
	}

	return goldap.ResultCodeSuccess, nil
}

// Modify request ------------------------

func (server *Bottin) HandleModify(ctx context.Context, w ldapserver.ResponseWriter, m *ldapserver.Message) context.Context {
	r := m.GetModifyRequest()

	code, err := server.handleModifyInternal(ctx, &r)

	res := ldapserver.NewResponse(code)
	if err != nil {
		res.SetDiagnosticMessage(err.Error())
	}
	if code == goldap.ResultCodeSuccess {
		server.logger.InfoContext(ctx, "successfully modified", "entry", r.Object())
	} else {
		server.logger.ErrorContext(ctx, "error while modifying entry", "entry", r.Object(), "err", err)
	}
	w.Write(goldap.ModifyResponse(res))
	return ctx
}

func (server *Bottin) handleModifyInternal(ctx context.Context, r *goldap.ModifyRequest) (int, error) {
	user := ldapserver.GetUser[User](ctx, EmptyUser)
	dn, err := server.parseDN(string(r.Object()), false)
	if err != nil {
		return goldap.ResultCodeInvalidDNSyntax, err
	}

	// First permission check with no particular attributes
	if !server.acl.Check(user, "modify", dn, []store.AttributeKey{}) &&
		!server.acl.Check(user, "modifyAdd", dn, []store.AttributeKey{}) {
		return goldap.ResultCodeInsufficientAccessRights, nil
	}

	server.logger.InfoContext(ctx, "modifying entry", "dn", dn)

	prevEntry, err := server.getEntry(ctx, dn)
	if err != nil {
		return goldap.ResultCodeOperationsError, err
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
			return goldap.ResultCodeObjectClassViolation, err
		}
		if attr.EqualFold(store.NewAttributeKey(dnFirstComponent.Type)) {
			return goldap.ResultCodeObjectClassViolation, fmt.Errorf("%s may not be changed as it is part of object path", attr)
		}

		// Check for permission to modify this attribute
		if !(server.acl.Check(user, "modify", dn, []store.AttributeKey{attr}) ||
			(change.Operation() == ldapserver.ModifyRequestChangeOperationAdd &&
				server.acl.Check(user, "modifyAdd", dn, []store.AttributeKey{attr}))) {
			return goldap.ResultCodeInsufficientAccessRights, nil
		}

		// If we are changing ATTR_MEMBER, rewrite all values to canonical form
		if attr.EqualFold(ATTR_MEMBER) {
			for i := range changeValues {
				canonicalVal, err := server.parseDN(changeValues[i], false)
				if err != nil {
					return goldap.ResultCodeInvalidDNSyntax, err
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
		if change.Operation() == ldapserver.ModifyRequestChangeOperationAdd {
			for _, val := range changeValues {
				if !slices.Contains(attrs[attr], val) {
					attrs[attr] = append(attrs[attr], val)
					if attr.EqualFold(ATTR_MEMBER) {
						valDN, err := server.parseDN(val, false)
						if err != nil {
							return goldap.ResultCodeInvalidDNSyntax, err
						}
						addMembers = append(addMembers, valDN)
					}
				}
			}
		} else if change.Operation() == ldapserver.ModifyRequestChangeOperationDelete {
			if len(changeValues) == 0 {
				// Delete everything
				if attr.EqualFold(ATTR_MEMBER) {
					for _, val := range attrs[attr] {
						valDN, err := server.parseDN(val, false)
						if err != nil {
							return goldap.ResultCodeInvalidDNSyntax, err
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
						if attr.EqualFold(ATTR_MEMBER) {
							valDN, err := server.parseDN(prevVal, false)
							if err != nil {
								return goldap.ResultCodeInvalidDNSyntax, err
							}
							delMembers = append(delMembers, valDN)
						}
					}
				}
				attrs[attr] = newList
			}
		} else if change.Operation() == ldapserver.ModifyRequestChangeOperationReplace {
			if attr.EqualFold(ATTR_MEMBER) {
				for _, newMem := range changeValues {
					if !slices.Contains(attrs[attr], newMem) {
						valDN, err := server.parseDN(newMem, false)
						if err != nil {
							return goldap.ResultCodeInvalidDNSyntax, err
						}
						addMembers = append(addMembers, valDN)
					}
				}
				for _, prevMem := range attrs[attr] {
					if !slices.Contains(changeValues, prevMem) {
						valDN, err := server.parseDN(prevMem, false)
						if err != nil {
							return goldap.ResultCodeInvalidDNSyntax, err
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
			return goldap.ResultCodeOperationsError, err
		}
		if !exists {
			return goldap.ResultCodeNoSuchObject, fmt.Errorf(
				"cannot add member %s, it does not exist", addMembers[i])
		}
	}

	for k, v := range attrs {
		if k.EqualFold(ATTR_OBJECTCLASS) && len(v) == 0 {
			return goldap.ResultCodeInsufficientAccessRights, fmt.Errorf(
				"cannot remove all objectclass values")
		}
	}

	// Now, the modification has been processed and accepted and we want to commit it
	attrs[ATTR_MODIFIERSNAME] = []string{user.user}
	attrs[ATTR_MODIFYTIMESTAMP] = []string{genTimestamp()}

	tx, err := server.store.Begin(ctx)
	if err != nil {
		return goldap.ResultCodeOperationsError, fmt.Errorf("error while beginning transaction: %w", err)
	}

	// Save the edited values
	entry := store.NewEntry(prevEntry.DN, attrs)
	if err = tx.Store(entry); err != nil {
		return goldap.ResultCodeOperationsError, err
	}

	// Update memberOf for added members and deleted members
	for _, addMem := range addMembers {
		if err := server.membershipAdd(tx, ATTR_MEMBEROF, addMem, dn); err != nil {
			return goldap.ResultCodeOperationsError, fmt.Errorf("error while adding memberOf: %w", err)
		}
	}

	for _, delMem := range delMembers {
		if err := server.membershipRemove(tx, ATTR_MEMBEROF, delMem, dn); err != nil {
			return goldap.ResultCodeOperationsError, fmt.Errorf("error while removing memberOf: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return goldap.ResultCodeOperationsError, fmt.Errorf("could not commit transaction: %w", err)
	}

	return goldap.ResultCodeSuccess, nil
}
