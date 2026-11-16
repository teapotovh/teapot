package store

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestMemStoreLifecycle(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	mem := NewMem()

	inbox, err := mem.CreateMailbox(ctx, "alice", "inbox")
	require.NoError(t, err)
	require.Equal(t, "INBOX", inbox.Name)

	_, err = mem.CreateMailbox(ctx, "alice", "INBOX")
	require.ErrorIs(t, err, ErrMailboxAlreadyExist)

	first, err := mem.Append(ctx, inbox.ID, []byte("Subject: one\r\n\r\nbody one\r\n"), nil, time.Time{})
	require.NoError(t, err)
	require.EqualValues(t, 1, first.UID)

	second, err := mem.Append(ctx, inbox.ID, []byte("Subject: two\r\n\r\nbody two\r\n"), []string{`\Seen`}, time.Time{})
	require.NoError(t, err)
	require.EqualValues(t, 2, second.UID)

	messages, err := mem.ListMessages(ctx, inbox.ID)
	require.NoError(t, err)
	require.Len(t, messages, 2)

	body, err := mem.Body(ctx, messages[0])
	require.NoError(t, err)
	require.Equal(t, "Subject: one\r\n\r\nbody one\r\n", string(body))

	updated, err := mem.StoreFlags(ctx, inbox.ID, []uint32{first.UID}, FlagAdd, []string{`\Flagged`})
	require.NoError(t, err)
	require.True(t, HasFlag(updated[0].Flags, `\Flagged`))

	archive, err := mem.CreateMailbox(ctx, "alice", "Archive")
	require.NoError(t, err)

	src, dst, err := mem.Copy(ctx, inbox.ID, archive.ID, []uint32{first.UID})
	require.NoError(t, err)
	require.Equal(t, []uint32{first.UID}, src)
	require.Len(t, dst, 1)

	movedSrc, movedDst, err := mem.Move(ctx, inbox.ID, archive.ID, []uint32{second.UID})
	require.NoError(t, err)
	require.Equal(t, []uint32{second.UID}, movedSrc)
	require.Len(t, movedDst, 1)

	require.NoError(t, mem.Expunge(ctx, inbox.ID, []uint32{first.UID}))
	messages, err = mem.ListMessages(ctx, inbox.ID)
	require.NoError(t, err)
	require.Empty(t, messages)
}
