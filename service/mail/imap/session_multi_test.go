package imap_test

import (
	"testing"
	"time"

	"github.com/emersion/go-imap/v2"

	"github.com/teapotovh/teapot/service/mail/store"
)

func TestIMAPSecondSessionSeesNewMessages(t *testing.T) {
	t.Parallel()

	srv, _ := newTestIMAPServer(t)
	firstClient, _ := dialClient(t, srv)
	secondClient, _ := dialClient(t, srv)

	if _, err := secondClient.Select(store.InboxName, nil).Wait(); err != nil {
		t.Fatalf("second select failed: %v", err)
	}

	if secondClient.Mailbox().NumMessages != 0 {
		t.Fatalf("expected empty inbox, got %d", secondClient.Mailbox().NumMessages)
	}

	appendMessage(t, firstClient, store.InboxName, testMessage1)

	deadline := time.Now().Add(3 * time.Second)

	for {
		if err := secondClient.Noop().Wait(); err != nil {
			t.Fatalf("noop failed: %v", err)
		}

		if secondClient.Mailbox().NumMessages == 1 {
			break
		}

		if time.Now().After(deadline) {
			t.Fatalf("second session did not see the new message, count=%d", secondClient.Mailbox().NumMessages)
		}

		time.Sleep(10 * time.Millisecond)
	}

	msgs, err := secondClient.Fetch(imap.UIDSetNum(1), &imap.FetchOptions{Flags: true}).Collect()
	if err != nil {
		t.Fatalf("second fetch failed: %v", err)
	}

	if len(msgs) != 1 {
		t.Fatalf("expected one message, got %d", len(msgs))
	}
}
