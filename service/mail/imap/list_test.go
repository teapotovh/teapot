package imap_test

import (
	"testing"

	"github.com/emersion/go-imap/v2"

	"github.com/teapotovh/teapot/service/mail/store"
)

func TestIMAPListWildcardsAndReference(t *testing.T) {
	t.Parallel()

	srv, _ := newTestIMAPServer(t)
	client, _ := dialClient(t, srv)

	for _, name := range []string{"Archive", "Archive/2025", "Archive/2026", "Sent"} {
		if err := client.Create(name, nil).Wait(); err != nil {
			t.Fatalf("could not create %q: %v", name, err)
		}
	}

	special, err := client.List("", "*", &imap.ListOptions{SelectSpecialUse: true}).Collect()
	if err != nil {
		t.Fatalf("list special use failed: %v", err)
	}

	if len(special) != 1 || special[0].Mailbox != store.InboxName {
		t.Fatalf("expected only INBOX as special use, got %#v", special)
	}

	all, err := client.List("", "*", nil).Collect()
	if err != nil {
		t.Fatalf("list all failed: %v", err)
	}

	if len(all) != 5 {
		t.Fatalf("expected 5 mailboxes, got %d", len(all))
	}

	children, err := client.List("", "Archive/%", nil).Collect()
	if err != nil {
		t.Fatalf("list children failed: %v", err)
	}

	if len(children) != 2 {
		t.Fatalf("expected 2 direct children, got %d", len(children))
	}

	referenced, err := client.List("Archive", "%", nil).Collect()
	if err != nil {
		t.Fatalf("list referenced failed: %v", err)
	}

	if len(referenced) != 2 {
		t.Fatalf("expected 2 referenced children, got %d", len(referenced))
	}

	subscribed, err := client.List("", "*", &imap.ListOptions{SelectSubscribed: true}).Collect()
	if err != nil {
		t.Fatalf("list subscribed failed: %v", err)
	}

	if len(subscribed) != 0 {
		t.Fatalf("expected no subscribed mailboxes, got %d", len(subscribed))
	}

	if err := client.Subscribe(store.InboxName).Wait(); err != nil {
		t.Fatalf("subscribe failed: %v", err)
	}

	subscribed, err = client.List("", "*", &imap.ListOptions{SelectSubscribed: true}).Collect()
	if err != nil {
		t.Fatalf("list subscribed failed: %v", err)
	}

	if len(subscribed) != 1 || subscribed[0].Mailbox != store.InboxName {
		t.Fatalf("expected subscribed INBOX, got %#v", subscribed)
	}
}
