package store

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	// DefaultDelimiter is the IMAP hierarchy delimiter.
	DefaultDelimiter = '/'

	// InboxName is the canonical name of the INBOX mailbox.
	InboxName = "INBOX"
)

var (
	ErrInvalidBackend      = errors.New("invalid backend")
	ErrNotFound            = errors.New("not found")
	ErrMailboxNotFound     = fmt.Errorf("mailbox: %w", ErrNotFound)
	ErrMessageNotFound     = fmt.Errorf("message: %w", ErrNotFound)
	ErrMailboxAlreadyExist = errors.New("mailbox already exists")
	ErrMailboxHasChildren  = errors.New("mailbox has children")
	ErrInvalidMailboxName  = errors.New("invalid mailbox name")
	ErrChecksumMismatch    = errors.New("message body checksum mismatch")
)

type Mailbox struct {
	ID            uuid.UUID
	Owner         string
	Name          string
	UIDValidity   uint32
	UIDNext       uint32
	HighestModSeq uint64
	ChangeSeq     int64
	Subscribed    bool
	SpecialUse    []string
	CreatedAt     time.Time
}

type Message struct {
	MailboxID    uuid.UUID
	UID          uint32
	Flags        []string
	ModSeq       uint64
	InternalDate time.Time
	Size         int64
	Digest       []byte
}

func CanonicalMailboxName(name string) string {
	name = strings.TrimSpace(name)
	if strings.EqualFold(name, InboxName) {
		return InboxName
	}

	return name
}

func ValidMailboxName(name string) bool {
	name = CanonicalMailboxName(name)
	if name == "" {
		return false
	}

	if strings.HasPrefix(name, string(DefaultDelimiter)) ||
		strings.HasSuffix(name, string(DefaultDelimiter)) {
		return false
	}

	return !strings.Contains(name, string(DefaultDelimiter)+string(DefaultDelimiter))
}

func NormalizeFlags(flags []string) []string {
	seen := make(map[string]struct{}, len(flags))
	result := make([]string, 0, len(flags))

	for _, flag := range flags {
		flag = strings.TrimSpace(flag)
		if flag == "" || strings.EqualFold(flag, `\Recent`) {
			continue
		}

		key := strings.ToLower(flag)
		if _, ok := seen[key]; ok {
			continue
		}

		seen[key] = struct{}{}

		result = append(result, flag)
	}

	return result
}

func HasFlag(flags []string, want string) bool {
	for _, flag := range flags {
		if strings.EqualFold(flag, want) {
			return true
		}
	}

	return false
}
