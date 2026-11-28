package store

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMemTransactionRollback(t *testing.T) {
	t.Parallel()

	m := NewMem()
	dn, err := ParseDN("uid=alice,ou=people,dc=bauble,dc=test")
	require.NoError(t, err)

	entry := NewEntry(dn, Attributes{testAttrObjectClass: {testObjectClassPerson}})

	tx, err := m.Begin(t.Context())
	require.NoError(t, err)
	require.NoError(t, tx.Store(t.Context(), entry))
	require.NoError(t, tx.Rollback(t.Context()))

	_, err = m.Get(t.Context(), dn.Prefix())
	require.ErrorIs(t, err, ErrNotFound)

	tx, err = m.Begin(t.Context())
	require.NoError(t, err)
	require.NoError(t, tx.Store(t.Context(), entry))
	require.NoError(t, tx.Commit(t.Context()))

	_, err = m.Get(t.Context(), dn.Prefix())
	require.NoError(t, err)
	require.ErrorIs(t, tx.Rollback(t.Context()), ErrCommitted)

	_, err = m.Get(t.Context(), dn.Prefix())
	require.NoError(t, err)
}
