package bottin

import (
	"fmt"
	"slices"

	"github.com/teapotovh/teapot/service/bottin/store"
)

func (server *Bottin) membershipAdd(
	tx store.Transaction,
	attr store.AttributeKey,
	member store.DN,
	group store.DN,
) error {
	entity, err := server.getEntry(tx.Context(), member)
	if err != nil {
		return fmt.Errorf("error while fetching membership %s value (adding): %w", attr, err)
	}

	membership := entity.Get(attr)
	if slices.Contains(membership, group.String()) {
		return nil
	}

	// Add group to the membership attribute
	membership = append(membership, group.String())

	entity.Attributes[attr] = membership
	if err = tx.Store(store.NewEntry(entity.DN, entity.Attributes)); err != nil {
		return fmt.Errorf("error while updating membership %s value (adding): %w", attr, err)
	}

	return nil
}

func (server *Bottin) membershipRemove(
	tx store.Transaction,
	attr store.AttributeKey,
	member store.DN,
	group store.DN,
) error {
	entity, err := server.getEntry(tx.Context(), member)
	if err != nil {
		return fmt.Errorf("error while fetching membership %s value (removing): %w", attr, err)
	}

	// Filter out group
	membership := entity.Get(attr)
	newMembership := []string{}

	for _, g := range membership {
		gdn, err := server.parseDN(g, false)
		if err != nil {
			return fmt.Errorf("error while parsing membership %s value (removing): %w", attr, err)
		}

		if !gdn.Equal(group) {
			newMembership = append(newMembership, gdn.String())
		}
	}

	if len(newMembership) == len(member) {
		return nil
	}

	// Update value of the membership attribute
	entity.Attributes[attr] = newMembership
	if err = tx.Store(store.NewEntry(entity.DN, entity.Attributes)); err != nil {
		return fmt.Errorf("error while updating membership %s value (removing): %w", attr, err)
	}

	return nil
}
