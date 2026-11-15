package store

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/teapotovh/teapot/lib/observability"
	"github.com/teapotovh/teapot/lib/run"
)

type memMailbox struct {
	mailbox  Mailbox
	messages map[uint32]*memMessage
}

type memMessage struct {
	message Message
	body    []byte
}

type Mem struct {
	mu       sync.RWMutex
	byID     map[uuid.UUID]*memMailbox
	byName   map[string]*memMailbox
	nextMod  uint64
	metrics  metrics
	notifier Notifier
}

func NewMem(notifier ...Notifier) *Mem {
	var n Notifier
	if len(notifier) > 0 {
		n = notifier[0]
	}

	m := &Mem{
		byID:     map[uuid.UUID]*memMailbox{},
		byName:   map[string]*memMailbox{},
		notifier: n,
	}
	m.metrics.initMetrics("mem")

	return m
}

func mailboxKey(owner, name string) string {
	return owner + "\x00" + CanonicalMailboxName(name)
}

func cloneMailbox(m Mailbox) Mailbox {
	m.SpecialUse = append([]string(nil), m.SpecialUse...)

	return m
}

func cloneMessage(m Message) Message {
	m.Flags = append([]string(nil), m.Flags...)
	m.Digest = append([]byte(nil), m.Digest...)

	return m
}

func (m *Mem) Ping(context.Context) error {
	return nil
}

func (m *Mem) ListMailboxes(_ context.Context, owner string) ([]Mailbox, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []Mailbox

	for _, box := range m.byID {
		if box.mailbox.Owner == owner {
			result = append(result, cloneMailbox(box.mailbox))
		}
	}

	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })

	return result, nil
}

func (m *Mem) GetMailbox(_ context.Context, owner, name string) (*Mailbox, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	box, ok := m.byName[mailboxKey(owner, name)]
	if !ok {
		return nil, ErrMailboxNotFound
	}

	mailbox := cloneMailbox(box.mailbox)

	return &mailbox, nil
}

func (m *Mem) CreateMailbox(_ context.Context, owner, name string, specialUse ...string) (*Mailbox, error) {
	name = CanonicalMailboxName(name)
	if !ValidMailboxName(name) {
		return nil, fmt.Errorf("%w: %q", ErrInvalidMailboxName, name)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	key := mailboxKey(owner, name)
	if _, ok := m.byName[key]; ok {
		return nil, ErrMailboxAlreadyExist
	}

	id, err := uuid.NewRandom()
	if err != nil {
		return nil, fmt.Errorf("error while generating mailbox id: %w", err)
	}

	box := &memMailbox{
		mailbox: Mailbox{
			ID:          id,
			Owner:       owner,
			Name:        name,
			UIDValidity: randomUIDValidity(),
			UIDNext:     1,
			SpecialUse:  append([]string(nil), specialUse...),
			CreatedAt:   time.Now().UTC(),
		},
		messages: map[uint32]*memMessage{},
	}

	m.byID[id] = box
	m.byName[key] = box

	mailbox := cloneMailbox(box.mailbox)

	return &mailbox, nil
}

func (m *Mem) DeleteMailbox(_ context.Context, owner, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := mailboxKey(owner, name)

	box, ok := m.byName[key]
	if !ok {
		return ErrMailboxNotFound
	}

	prefix := box.mailbox.Name + string(DefaultDelimiter)
	for _, other := range m.byID {
		if other.mailbox.Owner == owner && len(other.mailbox.Name) > len(prefix) &&
			other.mailbox.Name[:len(prefix)] == prefix {
			return ErrMailboxHasChildren
		}
	}

	delete(m.byName, key)
	delete(m.byID, box.mailbox.ID)

	return nil
}

func (m *Mem) RenameMailbox(_ context.Context, owner, oldName, newName string) error {
	newName = CanonicalMailboxName(newName)
	if !ValidMailboxName(newName) {
		return fmt.Errorf("%w: %q", ErrInvalidMailboxName, newName)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	oldKey := mailboxKey(owner, oldName)

	box, ok := m.byName[oldKey]
	if !ok {
		return ErrMailboxNotFound
	}

	newKey := mailboxKey(owner, newName)
	if _, ok := m.byName[newKey]; ok {
		return ErrMailboxAlreadyExist
	}

	delete(m.byName, oldKey)

	oldPrefix := box.mailbox.Name + string(DefaultDelimiter)
	newPrefix := newName + string(DefaultDelimiter)

	for key, other := range m.byName {
		if other.mailbox.Owner != owner || len(other.mailbox.Name) <= len(oldPrefix) {
			continue
		}

		if other.mailbox.Name[:len(oldPrefix)] != oldPrefix {
			continue
		}

		delete(m.byName, key)

		other.mailbox.Name = newPrefix + other.mailbox.Name[len(oldPrefix):]
		m.byName[mailboxKey(owner, other.mailbox.Name)] = other
	}

	box.mailbox.Name = newName
	m.byName[newKey] = box

	return nil
}

func (m *Mem) SetSubscribed(_ context.Context, owner, name string, subscribed bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	box, ok := m.byName[mailboxKey(owner, name)]
	if !ok {
		return ErrMailboxNotFound
	}

	box.mailbox.Subscribed = subscribed

	return nil
}

func (m *Mem) ListMessages(_ context.Context, mailboxID uuid.UUID) ([]Message, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	box, ok := m.byID[mailboxID]
	if !ok {
		return nil, ErrMailboxNotFound
	}

	result := make([]Message, 0, len(box.messages))
	for _, msg := range box.messages {
		result = append(result, cloneMessage(msg.message))
	}

	sort.Slice(result, func(i, j int) bool { return result[i].UID < result[j].UID })

	return result, nil
}

func (m *Mem) GetMessages(_ context.Context, mailboxID uuid.UUID, uids []uint32) ([]Message, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	box, ok := m.byID[mailboxID]
	if !ok {
		return nil, ErrMailboxNotFound
	}

	var result []Message

	for _, uid := range uids {
		if msg, ok := box.messages[uid]; ok {
			result = append(result, cloneMessage(msg.message))
		}
	}

	sort.Slice(result, func(i, j int) bool { return result[i].UID < result[j].UID })

	return result, nil
}

func (m *Mem) Body(_ context.Context, message Message) ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	box, ok := m.byID[message.MailboxID]
	if !ok {
		return nil, ErrMailboxNotFound
	}

	msg, ok := box.messages[message.UID]
	if !ok {
		return nil, ErrMessageNotFound
	}

	return append([]byte(nil), msg.body...), nil
}

//nolint:funcorder
func (m *Mem) nextModSeqLocked() uint64 {
	m.nextMod++
	return m.nextMod
}

func (m *Mem) Append(
	_ context.Context,
	mailboxID uuid.UUID,
	data []byte,
	flags []string,
	internalDate time.Time,
) (*Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	box, ok := m.byID[mailboxID]
	if !ok {
		return nil, ErrMailboxNotFound
	}

	if internalDate.IsZero() {
		internalDate = time.Now().UTC()
	}

	modseq := m.nextModSeqLocked()
	uid := box.mailbox.UIDNext
	box.mailbox.UIDNext++
	box.mailbox.HighestModSeq = modseq
	box.mailbox.ChangeSeq++

	msg := &memMessage{
		message: Message{
			MailboxID:    mailboxID,
			UID:          uid,
			Flags:        NormalizeFlags(flags),
			ModSeq:       modseq,
			InternalDate: internalDate.UTC(),
			Size:         int64(len(data)),
			Digest:       digestBytes(data),
		},
		body: append([]byte(nil), data...),
	}
	box.messages[uid] = msg

	result := cloneMessage(msg.message)

	m.publish(mailboxID)

	return &result, nil
}

func (m *Mem) StoreFlags(
	_ context.Context,
	mailboxID uuid.UUID,
	uids []uint32,
	op FlagOp,
	flags []string,
) ([]Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	box, ok := m.byID[mailboxID]
	if !ok {
		return nil, ErrMailboxNotFound
	}

	wanted := NormalizeFlags(flags)
	modseq := m.nextModSeqLocked()
	box.mailbox.HighestModSeq = modseq
	box.mailbox.ChangeSeq++

	var result []Message

	for _, uid := range uids {
		msg, ok := box.messages[uid]
		if !ok {
			continue
		}

		switch op {
		case FlagSet:
			msg.message.Flags = wanted
		case FlagAdd:
			msg.message.Flags = NormalizeFlags(append(msg.message.Flags, wanted...))
		case FlagDel:
			var kept []string

			for _, flag := range msg.message.Flags {
				remove := false

				for _, del := range wanted {
					if equalFold(flag, del) {
						remove = true
						break
					}
				}

				if !remove {
					kept = append(kept, flag)
				}
			}

			msg.message.Flags = kept
		}

		msg.message.ModSeq = modseq
		result = append(result, cloneMessage(msg.message))
	}

	m.publish(mailboxID)

	return result, nil
}

func (m *Mem) Expunge(_ context.Context, mailboxID uuid.UUID, uids []uint32) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	box, ok := m.byID[mailboxID]
	if !ok {
		return ErrMailboxNotFound
	}

	box.mailbox.ChangeSeq++
	for _, uid := range uids {
		delete(box.messages, uid)
	}

	m.publish(mailboxID)

	return nil
}

func (m *Mem) Copy(_ context.Context, sourceID, destinationID uuid.UUID, uids []uint32) ([]uint32, []uint32, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	source, ok := m.byID[sourceID]
	if !ok {
		return nil, nil, ErrMailboxNotFound
	}

	dest, ok := m.byID[destinationID]
	if !ok {
		return nil, nil, ErrMailboxNotFound
	}

	modseq := m.nextModSeqLocked()
	dest.mailbox.HighestModSeq = modseq
	dest.mailbox.ChangeSeq++

	var sourceUIDs, destUIDs []uint32

	for _, uid := range uids {
		msg, ok := source.messages[uid]
		if !ok {
			continue
		}

		newUID := dest.mailbox.UIDNext
		dest.mailbox.UIDNext++

		copied := &memMessage{
			message: cloneMessage(msg.message),
			body:    append([]byte(nil), msg.body...),
		}
		copied.message.MailboxID = destinationID
		copied.message.UID = newUID
		copied.message.ModSeq = modseq
		dest.messages[newUID] = copied

		sourceUIDs = append(sourceUIDs, uid)
		destUIDs = append(destUIDs, newUID)
	}

	m.publish(destinationID)

	return sourceUIDs, destUIDs, nil
}

func (m *Mem) Move(ctx context.Context, sourceID, destinationID uuid.UUID, uids []uint32) ([]uint32, []uint32, error) {
	sourceUIDs, destUIDs, err := m.Copy(ctx, sourceID, destinationID, uids)
	if err != nil {
		return nil, nil, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	source := m.byID[sourceID]

	source.mailbox.ChangeSeq++
	for _, uid := range sourceUIDs {
		delete(source.messages, uid)
	}

	m.publish(sourceID)
	m.publish(destinationID)

	return sourceUIDs, destUIDs, nil
}

func (m *Mem) Run(ctx context.Context, notify run.Notify) error {
	notify.Notify()
	<-ctx.Done()

	return nil
}

func (m *Mem) Metrics() []prometheus.Collector {
	return []prometheus.Collector{m.metrics.backend}
}

func (m *Mem) ReadinessChecks() map[string]observability.Check {
	return map[string]observability.Check{}
}

func equalFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}

	for i := range a {
		if lower(a[i]) != lower(b[i]) {
			return false
		}
	}

	return true
}

func lower(b byte) byte {
	if b >= 'A' && b <= 'Z' {
		return b + ('a' - 'A')
	}

	return b
}

var (
	_ Store                         = (*Mem)(nil)
	_ run.Runnable                  = (*Mem)(nil)
	_ observability.Metrics         = (*Mem)(nil)
	_ observability.ReadinessChecks = (*Mem)(nil)
)

//nolint:funcorder
func (m *Mem) publish(mailboxID uuid.UUID) {
	if m.notifier != nil {
		m.notifier.Publish(mailboxID)
	}
}

func (m *Mem) SetNotifier(notifier Notifier) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.notifier = notifier
}
