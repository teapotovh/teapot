package imap_test

import (
	"testing"
	"time"

	"github.com/teapotovh/teapot/service/mail/store"
)

func TestIMAPIdleReceivesNotification(t *testing.T) {
	t.Parallel()

	srv, _ := newTestIMAPServer(t)
	firstClient, _ := dialClient(t, srv)
	secondClient, _ := dialClient(t, srv)

	if _, err := secondClient.Select(store.InboxName, nil).Wait(); err != nil {
		t.Fatalf("second select failed: %v", err)
	}

	idle, err := secondClient.Idle()
	if err != nil {
		t.Fatalf("could not start IDLE: %v", err)
	}

	appendMessage(t, firstClient, store.InboxName, testMessage1)

	deadline := time.Now().Add(3 * time.Second)
	for secondClient.Mailbox().NumMessages != 1 {
		if time.Now().After(deadline) {
			t.Fatalf("IDLE did not deliver EXISTS, count=%d", secondClient.Mailbox().NumMessages)
		}

		time.Sleep(10 * time.Millisecond)
	}

	if err := idle.Close(); err != nil {
		t.Fatalf("could not stop IDLE: %v", err)
	}

	if err := idle.Wait(); err != nil {
		t.Fatalf("IDLE failed: %v", err)
	}
}
