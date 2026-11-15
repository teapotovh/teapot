package imap_test

import (
	"testing"

	"github.com/emersion/go-imap/v2"

	"github.com/teapotovh/teapot/service/mail/store"
)

func TestIMAPSeenSideEffectsAndExpungeOrder(t *testing.T) {
	t.Parallel()

	srv, _ := newTestIMAPServer(t)
	client, _ := dialClient(t, srv)

	first := appendMessage(t, client, store.InboxName, testMessage1)
	second := appendMessage(t, client, store.InboxName, testMessage2)
	third := appendMessage(t, client, store.InboxName, "Subject: third\r\n\r\nthird\r\n")

	if _, err := client.Select(store.InboxName, nil).Wait(); err != nil {
		t.Fatalf("select failed: %v", err)
	}

	if _, err := client.Fetch(imap.UIDSetNum(first.UID), &imap.FetchOptions{
		BodySection: []*imap.FetchItemBodySection{{Peek: true}},
	}).Collect(); err != nil {
		t.Fatalf("peek fetch failed: %v", err)
	}

	flags, err := client.Fetch(imap.UIDSetNum(first.UID), &imap.FetchOptions{Flags: true}).Collect()
	if err != nil {
		t.Fatalf("flag fetch failed: %v", err)
	}

	if hasFlag(flags[0].Flags, imap.FlagSeen) {
		t.Fatal("BODY.PEEK must not set \\Seen")
	}

	if _, err := client.Fetch(imap.UIDSetNum(second.UID), &imap.FetchOptions{
		BodySection: []*imap.FetchItemBodySection{{}},
	}).Collect(); err != nil {
		t.Fatalf("body fetch failed: %v", err)
	}

	flags, err = client.Fetch(imap.UIDSetNum(second.UID), &imap.FetchOptions{Flags: true}).Collect()
	if err != nil {
		t.Fatalf("flag fetch failed: %v", err)
	}

	if !hasFlag(flags[0].Flags, imap.FlagSeen) {
		t.Fatal("BODY[] must set \\Seen")
	}

	for _, uid := range []imap.UID{first.UID, second.UID} {
		if err := client.Store(imap.UIDSetNum(uid), &imap.StoreFlags{
			Op:     imap.StoreFlagsAdd,
			Silent: true,
			Flags:  []imap.Flag{imap.FlagDeleted},
		}, nil).Close(); err != nil {
			t.Fatalf("mark deleted failed: %v", err)
		}
	}

	expunged, err := client.Expunge().Collect()
	if err != nil {
		t.Fatalf("expunge failed: %v", err)
	}

	if len(expunged) != 2 || expunged[0] != 2 || expunged[1] != 1 {
		t.Fatalf("expected descending expunge sequence numbers [2 1], got %v", expunged)
	}

	remaining, err := client.Fetch(imap.SeqSetNum(1), &imap.FetchOptions{UID: true}).Collect()
	if err != nil {
		t.Fatalf("fetch remaining failed: %v", err)
	}

	if len(remaining) != 1 || remaining[0].UID != third.UID {
		t.Fatalf("expected UID %d at sequence 1, got %#v", third.UID, remaining)
	}
}
