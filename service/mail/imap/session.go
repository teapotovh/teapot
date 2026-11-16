package imap

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapserver"
	"github.com/emersion/go-message/textproto"
	"github.com/google/uuid"

	"github.com/teapotovh/teapot/lib/ldap"
	mailservice "github.com/teapotovh/teapot/service/mail"
	"github.com/teapotovh/teapot/service/mail/store"
)

const defaultDelimiter = '/'

var systemFlags = []imap.Flag{
	imap.FlagSeen,
	imap.FlagAnswered,
	imap.FlagFlagged,
	imap.FlagDeleted,
	imap.FlagDraft,
}

type Session struct {
	logger *slog.Logger
	store  store.Store
	auth   ldap.Authenticator

	user      *ldap.User
	mailbox   *store.Mailbox
	view      *View
	flags     map[uint32][]string
	readOnly  bool
	condstore bool

	pollInterval time.Duration
	watch        <-chan uuid.UUID
	unsubscribe  func()
}

func NewSession(mail *mailservice.Mail, logger *slog.Logger, pollInterval time.Duration) *Session {
	if pollInterval <= 0 {
		pollInterval = defaultPollInterval
	}

	session := &Session{
		logger:       logger,
		store:        mail.Store(),
		auth:         mail.Authenticator(),
		flags:        map[uint32][]string{},
		pollInterval: pollInterval,
	}

	if mail != nil {
		session.watch, session.unsubscribe = mail.Subscribe()
	}

	return session
}

func (s *Session) Close() error {
	if s.unsubscribe != nil {
		s.unsubscribe()
		s.unsubscribe = nil
	}

	s.mailbox = nil
	s.view = nil
	s.flags = map[uint32][]string{}

	return nil
}

func (s *Session) Login(username, password string) error {
	user, err := s.auth.Authenticate(context.Background(), username, password)
	if err != nil {
		return imapserver.ErrAuthFailed
	}

	s.user = user

	if _, err := s.store.GetMailbox(
		context.Background(),
		user.Username,
		store.InboxName,
	); errors.Is(
		err,
		store.ErrMailboxNotFound,
	) {
		if _, err := s.store.CreateMailbox(
			context.Background(),
			user.Username,
			store.InboxName,
			"\\Inbox",
		); err != nil &&
			!errors.Is(err, store.ErrMailboxAlreadyExist) {
			return fmt.Errorf("error while creating the inbox: %w", err)
		}
	}

	return nil
}

//nolint:funcorder
func (s *Session) requireUser() error {
	if s.user == nil {
		return &imap.Error{Type: imap.StatusResponseTypeNo, Text: "Not authenticated"}
	}

	return nil
}

func (s *Session) Select(name string, options *imap.SelectOptions) (*imap.SelectData, error) {
	if err := s.requireUser(); err != nil {
		return nil, err
	}

	mailbox, err := s.store.GetMailbox(context.Background(), s.user.Username, name)
	if err != nil {
		if name == store.InboxName || strings.EqualFold(name, store.InboxName) {
			mailbox, err = s.store.CreateMailbox(context.Background(), s.user.Username, store.InboxName, "\\Inbox")
			if errors.Is(err, store.ErrMailboxAlreadyExist) {
				mailbox, err = s.store.GetMailbox(context.Background(), s.user.Username, store.InboxName)
			}
		}

		if err != nil {
			return nil, &imap.Error{Type: imap.StatusResponseTypeNo, Text: "Mailbox does not exist"}
		}
	}

	messages, err := s.store.ListMessages(context.Background(), mailbox.ID)
	if err != nil {
		return nil, err
	}

	uids := make([]uint32, 0, len(messages))
	s.flags = make(map[uint32][]string, len(messages))

	for _, msg := range messages {
		uids = append(uids, msg.UID)
		s.flags[msg.UID] = append([]string(nil), msg.Flags...)
	}

	s.mailbox = mailbox
	s.view = NewView(mailbox.UIDValidity, uids)
	s.readOnly = options.ReadOnly
	s.condstore = options.CondStore

	firstUnseen := uint32(0)

	for _, msg := range messages {
		if !store.HasFlag(msg.Flags, `\Seen`) {
			firstUnseen, _ = s.view.Seq(msg.UID)
			break
		}
	}

	return &imap.SelectData{
		Flags:             append([]imap.Flag(nil), systemFlags...),
		PermanentFlags:    append([]imap.Flag(nil), systemFlags...),
		NumMessages:       s.view.Count(),
		NumRecent:         0,
		UIDNext:           imap.UID(mailbox.UIDNext),
		UIDValidity:       mailbox.UIDValidity,
		FirstUnseenSeqNum: firstUnseen,
		HighestModSeq:     mailbox.HighestModSeq,
		ReadOnly:          options.ReadOnly,
	}, nil
}

func (s *Session) Create(name string, options *imap.CreateOptions) error {
	if err := s.requireUser(); err != nil {
		return err
	}

	var specialUse []string

	if options != nil {
		for _, attr := range options.SpecialUse {
			specialUse = append(specialUse, string(attr))
		}
	}

	_, err := s.store.CreateMailbox(context.Background(), s.user.Username, name, specialUse...)
	if errors.Is(err, store.ErrMailboxAlreadyExist) {
		return &imap.Error{Type: imap.StatusResponseTypeNo, Text: "Mailbox already exists"}
	}

	return err
}

func (s *Session) Delete(name string) error {
	if err := s.requireUser(); err != nil {
		return err
	}

	err := s.store.DeleteMailbox(context.Background(), s.user.Username, name)
	if errors.Is(err, store.ErrMailboxNotFound) {
		return &imap.Error{Type: imap.StatusResponseTypeNo, Text: "Mailbox does not exist"}
	}

	return err
}

func (s *Session) Rename(oldName, newName string, _ *imap.RenameOptions) error {
	if err := s.requireUser(); err != nil {
		return err
	}

	return s.store.RenameMailbox(context.Background(), s.user.Username, oldName, newName)
}

func (s *Session) Subscribe(name string) error {
	if err := s.requireUser(); err != nil {
		return err
	}

	return s.store.SetSubscribed(context.Background(), s.user.Username, name, true)
}

func (s *Session) Unsubscribe(name string) error {
	if err := s.requireUser(); err != nil {
		return err
	}

	return s.store.SetSubscribed(context.Background(), s.user.Username, name, false)
}

func (s *Session) List(w *imapserver.ListWriter, ref string, patterns []string, options *imap.ListOptions) error {
	if err := s.requireUser(); err != nil {
		return err
	}

	mailboxes, err := s.store.ListMailboxes(context.Background(), s.user.Username)
	if err != nil {
		return err
	}

	names := make(map[string]struct{}, len(mailboxes))
	for _, box := range mailboxes {
		names[box.Name] = struct{}{}
	}

	for _, box := range mailboxes {
		if options.SelectSubscribed && !box.Subscribed {
			continue
		}

		if options.SelectSpecialUse && len(box.SpecialUse) == 0 {
			continue
		}

		if !matchListPatterns(ref, box.Name, patterns) {
			continue
		}

		attrs := mailboxAttrs(box, names)
		if options.ReturnSubscribed && box.Subscribed {
			attrs = append(attrs, imap.MailboxAttrSubscribed)
		}

		data := &imap.ListData{
			Attrs:   attrs,
			Delim:   defaultDelimiter,
			Mailbox: box.Name,
		}

		if options.ReturnStatus != nil {
			status, err := s.statusFor(box, options.ReturnStatus)
			if err != nil {
				return err
			}

			data.Status = status
		}

		if err := w.WriteList(data); err != nil {
			return err
		}
	}

	return nil
}

func (s *Session) Status(name string, options *imap.StatusOptions) (*imap.StatusData, error) {
	if err := s.requireUser(); err != nil {
		return nil, err
	}

	box, err := s.store.GetMailbox(context.Background(), s.user.Username, name)
	if err != nil {
		return nil, &imap.Error{Type: imap.StatusResponseTypeNo, Text: "Mailbox does not exist"}
	}

	return s.statusFor(*box, options)
}

//nolint:funcorder
func (s *Session) statusFor(box store.Mailbox, options *imap.StatusOptions) (*imap.StatusData, error) {
	messages, err := s.store.ListMessages(context.Background(), box.ID)
	if err != nil {
		return nil, err
	}

	data := &imap.StatusData{Mailbox: box.Name, UIDNext: imap.UID(box.UIDNext), UIDValidity: box.UIDValidity}
	num := uint32(len(messages)) //nolint:gosec
	unseen := uint32(0)
	deleted := uint32(0)

	var size int64 = 0

	for _, msg := range messages {
		size += msg.Size
		if !store.HasFlag(msg.Flags, `\Seen`) {
			unseen++
		}

		if store.HasFlag(msg.Flags, `\Deleted`) {
			deleted++
		}
	}

	if options.NumMessages {
		data.NumMessages = &num
	}

	if options.NumUnseen {
		data.NumUnseen = &unseen
	}

	if options.NumDeleted {
		data.NumDeleted = &deleted
	}

	if options.Size {
		data.Size = &size
	}

	if options.NumRecent {
		zero := uint32(0)
		data.NumRecent = &zero
	}

	if options.HighestModSeq {
		data.HighestModSeq = box.HighestModSeq
	}

	return data, nil
}

func (s *Session) Append(mailbox string, r imap.LiteralReader, options *imap.AppendOptions) (*imap.AppendData, error) {
	if err := s.requireUser(); err != nil {
		return nil, err
	}

	data := make([]byte, r.Size())
	if _, err := io.ReadFull(r, data); err != nil {
		return nil, err
	}

	box, err := s.store.GetMailbox(context.Background(), s.user.Username, mailbox)
	if err != nil {
		return nil, &imap.Error{Type: imap.StatusResponseTypeNo, Text: "Mailbox does not exist"}
	}

	var flags []string
	for _, flag := range options.Flags {
		flags = append(flags, string(flag))
	}

	msg, err := s.store.Append(context.Background(), box.ID, data, flags, options.Time)
	if err != nil {
		return nil, err
	}

	return &imap.AppendData{UID: imap.UID(msg.UID), UIDValidity: box.UIDValidity}, nil
}

func (s *Session) Poll(w *imapserver.UpdateWriter, allowExpunge bool) error {
	return s.sync(w, allowExpunge)
}

func (s *Session) Idle(w *imapserver.UpdateWriter, stop <-chan struct{}) error {
	ticker := time.NewTicker(s.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-stop:
			return nil
		case <-ticker.C:
			if err := s.sync(w, true); err != nil {
				return err
			}
		case mailboxID := <-s.watch:
			if s.mailbox == nil || mailboxID != s.mailbox.ID {
				continue
			}

			if err := s.sync(w, true); err != nil {
				return err
			}
		}
	}
}

//nolint:gocyclo,funcorder
func (s *Session) sync(w *imapserver.UpdateWriter, allowExpunge bool) error {
	if s.mailbox == nil || s.view == nil {
		return nil
	}

	messages, err := s.store.ListMessages(context.Background(), s.mailbox.ID)
	if err != nil {
		return err
	}

	current := make(map[uint32]store.Message, len(messages))

	currentUIDs := make(map[uint32]struct{}, len(messages))
	for _, msg := range messages {
		current[msg.UID] = msg
		currentUIDs[msg.UID] = struct{}{}
	}

	if allowExpunge {
		var expunged []uint32

		for _, uid := range s.view.UIDs() {
			if _, ok := currentUIDs[uid]; !ok {
				if seq, ok := s.view.Seq(uid); ok {
					expunged = append(expunged, seq)
				}
			}
		}

		for _, uid := range s.view.UIDs() {
			if _, ok := currentUIDs[uid]; !ok {
				_, _ = s.view.Remove(uid)
				delete(s.flags, uid)
			}
		}

		sort.Slice(expunged, func(i, j int) bool { return expunged[i] > expunged[j] })

		for _, seq := range expunged {
			if err := w.WriteExpunge(seq); err != nil {
				return err
			}
		}
	}

	appended := false
	oldHighest := s.view.HighestUID()

	for _, msg := range messages {
		if _, alive := s.view.Seq(msg.UID); alive {
			old := s.flags[msg.UID]
			if !sameFlags(old, msg.Flags) {
				s.flags[msg.UID] = append([]string(nil), msg.Flags...)

				seq, _ := s.view.Seq(msg.UID)
				if err := w.WriteMessageFlags(seq, imap.UID(msg.UID), toIMAPFlags(msg.Flags)); err != nil {
					return err
				}
			}

			continue
		}

		if msg.UID <= oldHighest {
			continue
		}

		if err := s.view.Append(msg.UID); err != nil {
			return err
		}

		s.flags[msg.UID] = append([]string(nil), msg.Flags...)
		appended = true
	}

	if appended {
		return w.WriteNumMessages(s.view.Count())
	}

	return nil
}

func (s *Session) Unselect() error {
	s.mailbox = nil
	s.view = nil
	s.flags = map[uint32][]string{}

	return nil
}

//nolint:gocyclo
func (s *Session) Expunge(w *imapserver.ExpungeWriter, uids *imap.UIDSet) error {
	if s.readOnly {
		return &imap.Error{Type: imap.StatusResponseTypeNo, Text: "Mailbox is read-only"}
	}

	if s.mailbox == nil || s.view == nil {
		return &imap.Error{Type: imap.StatusResponseTypeBad, Text: "No mailbox selected"}
	}

	messages, err := s.store.GetMessages(context.Background(), s.mailbox.ID, s.view.UIDs())
	if err != nil {
		return err
	}

	var expunge []uint32

	for _, msg := range messages {
		if !store.HasFlag(msg.Flags, `\Deleted`) {
			continue
		}

		if uids != nil && !uids.Contains(imap.UID(msg.UID)) {
			continue
		}

		expunge = append(expunge, msg.UID)
	}

	if len(expunge) == 0 {
		return nil
	}

	if err := s.store.Expunge(context.Background(), s.mailbox.ID, expunge); err != nil {
		return err
	}

	var seqs []uint32

	for _, uid := range expunge {
		if seq, ok := s.view.Seq(uid); ok {
			seqs = append(seqs, seq)
		}
	}

	for _, uid := range expunge {
		_, _ = s.view.Remove(uid)
		delete(s.flags, uid)
	}

	sort.Slice(seqs, func(i, j int) bool { return seqs[i] > seqs[j] })

	for _, seq := range seqs {
		if w != nil {
			if err := w.WriteExpunge(seq); err != nil {
				return err
			}
		}
	}

	return nil
}

func (s *Session) Search(
	numKind imapserver.NumKind,
	criteria *imap.SearchCriteria,
	options *imap.SearchOptions,
) (*imap.SearchData, error) {
	if s.mailbox == nil || s.view == nil {
		return nil, &imap.Error{Type: imap.StatusResponseTypeBad, Text: "No mailbox selected"}
	}

	messages, err := s.store.ListMessages(context.Background(), s.mailbox.ID)
	if err != nil {
		return nil, err
	}

	needBody := searchNeedsBody(criteria)

	var (
		seqSet imap.SeqSet
		uidSet imap.UIDSet
		data   imap.SearchData
	)

	for _, msg := range messages {
		seq, ok := s.view.Seq(msg.UID)
		if !ok {
			continue
		}

		var body []byte
		if needBody {
			body, err = s.store.Body(context.Background(), msg)
			if err != nil {
				return nil, err
			}
		}

		searchMsg := SearchMessage{
			UID:          msg.UID,
			InternalDate: msg.InternalDate,
			Size:         msg.Size,
			Flags:        msg.Flags,
			ModSeq:       msg.ModSeq,
			Body:         body,
		}

		if !MatchSearch(seq, searchMsg, criteria) {
			continue
		}

		uidSet.AddNum(imap.UID(msg.UID))

		var num uint32
		if numKind == imapserver.NumKindSeq {
			num = seq
			seqSet.AddNum(seq)
		} else {
			num = msg.UID
		}

		if data.Min == 0 || num < data.Min {
			data.Min = num
		}

		if num > data.Max {
			data.Max = num
		}

		data.Count++
	}

	if numKind == imapserver.NumKindSeq {
		data.All = seqSet
	} else {
		data.All = uidSet
	}

	return &data, nil
}

//nolint:gocyclo
func (s *Session) Fetch(w *imapserver.FetchWriter, numSet imap.NumSet, options *imap.FetchOptions) error {
	if s.mailbox == nil || s.view == nil {
		return &imap.Error{Type: imap.StatusResponseTypeBad, Text: "No mailbox selected"}
	}

	uids, err := s.resolve(numSet)
	if err != nil {
		return err
	}

	messages, err := s.store.GetMessages(context.Background(), s.mailbox.ID, uids)
	if err != nil {
		return err
	}

	markSeen := false

	for _, bs := range options.BodySection {
		if !bs.Peek {
			markSeen = true
		}
	}

	for _, bs := range options.BinarySection {
		if !bs.Peek {
			markSeen = true
		}
	}

	for _, msg := range messages {
		seq, ok := s.view.Seq(msg.UID)
		if !ok {
			continue
		}

		flags := msg.Flags
		if markSeen && !store.HasFlag(flags, `\Seen`) {
			updated, err := s.store.StoreFlags(
				context.Background(),
				s.mailbox.ID,
				[]uint32{msg.UID},
				store.FlagAdd,
				[]string{`\Seen`},
			)
			if err != nil {
				return err
			}

			if len(updated) > 0 {
				flags = updated[0].Flags
				s.flags[msg.UID] = append([]string(nil), flags...)
			}
		}

		var body []byte
		if fetchNeedsBody(options) {
			body, err = s.store.Body(context.Background(), msg)
			if err != nil {
				return err
			}
		}

		resp := w.CreateMessage(seq)
		if options.UID {
			resp.WriteUID(imap.UID(msg.UID))
		}

		if options.Flags {
			resp.WriteFlags(toIMAPFlags(flags))
		}

		if options.InternalDate {
			resp.WriteInternalDate(msg.InternalDate)
		}

		if options.RFC822Size {
			resp.WriteRFC822Size(msg.Size)
		}

		if options.Envelope {
			header, _ := textproto.ReadHeader(bufio.NewReader(bytes.NewReader(body)))
			resp.WriteEnvelope(imapserver.ExtractEnvelope(header))
		}

		if options.BodyStructure != nil {
			resp.WriteBodyStructure(imapserver.ExtractBodyStructure(bytes.NewReader(body)))
		}

		for _, section := range options.BodySection {
			data := imapserver.ExtractBodySection(bytes.NewReader(body), section)

			wc := resp.WriteBodySection(section, int64(len(data)))
			if _, err := wc.Write(data); err != nil {
				_ = wc.Close()
				_ = resp.Close()

				return err
			}

			if err := wc.Close(); err != nil {
				_ = resp.Close()
				return err
			}
		}

		for _, section := range options.BinarySection {
			data := imapserver.ExtractBinarySection(bytes.NewReader(body), section)

			wc := resp.WriteBinarySection(section, int64(len(data)))
			if _, err := wc.Write(data); err != nil {
				_ = wc.Close()
				_ = resp.Close()

				return err
			}

			if err := wc.Close(); err != nil {
				_ = resp.Close()
				return err
			}
		}

		for _, section := range options.BinarySectionSize {
			resp.WriteBinarySectionSize(section, imapserver.ExtractBinarySectionSize(bytes.NewReader(body), section))
		}

		if err := resp.Close(); err != nil {
			return err
		}
	}

	return nil
}

func (s *Session) Store(
	w *imapserver.FetchWriter,
	numSet imap.NumSet,
	flags *imap.StoreFlags,
	_ *imap.StoreOptions,
) error {
	if s.readOnly {
		return &imap.Error{Type: imap.StatusResponseTypeNo, Text: "Mailbox is read-only"}
	}

	if s.mailbox == nil || s.view == nil {
		return &imap.Error{Type: imap.StatusResponseTypeBad, Text: "No mailbox selected"}
	}

	uids, err := s.resolve(numSet)
	if err != nil {
		return err
	}

	var op store.FlagOp

	switch flags.Op {
	case imap.StoreFlagsSet:
		op = store.FlagSet
	case imap.StoreFlagsAdd:
		op = store.FlagAdd
	case imap.StoreFlagsDel:
		op = store.FlagDel
	}

	var raw []string
	for _, flag := range flags.Flags {
		raw = append(raw, string(flag))
	}

	updated, err := s.store.StoreFlags(context.Background(), s.mailbox.ID, uids, op, raw)
	if err != nil {
		return err
	}

	for _, msg := range updated {
		s.flags[msg.UID] = append([]string(nil), msg.Flags...)
	}

	if flags.Silent {
		return nil
	}

	for _, msg := range updated {
		seq, ok := s.view.Seq(msg.UID)
		if !ok {
			continue
		}

		resp := w.CreateMessage(seq)
		resp.WriteUID(imap.UID(msg.UID))
		resp.WriteFlags(toIMAPFlags(msg.Flags))

		if err := resp.Close(); err != nil {
			return err
		}
	}

	return nil
}

func (s *Session) Copy(numSet imap.NumSet, dest string) (*imap.CopyData, error) {
	if s.mailbox == nil || s.view == nil {
		return nil, &imap.Error{Type: imap.StatusResponseTypeBad, Text: "No mailbox selected"}
	}

	uids, err := s.resolve(numSet)
	if err != nil {
		return nil, err
	}

	destBox, err := s.store.GetMailbox(context.Background(), s.user.Username, dest)
	if err != nil {
		return nil, &imap.Error{Type: imap.StatusResponseTypeNo, Text: "Mailbox does not exist"}
	}

	sourceUIDs, destUIDs, err := s.store.Copy(context.Background(), s.mailbox.ID, destBox.ID, uids)
	if err != nil {
		return nil, err
	}

	return copyData(destBox.UIDValidity, sourceUIDs, destUIDs), nil
}

func (s *Session) Move(w *imapserver.MoveWriter, numSet imap.NumSet, dest string) error {
	if s.readOnly {
		return &imap.Error{Type: imap.StatusResponseTypeNo, Text: "Mailbox is read-only"}
	}

	if s.mailbox == nil || s.view == nil {
		return &imap.Error{Type: imap.StatusResponseTypeBad, Text: "No mailbox selected"}
	}

	uids, err := s.resolve(numSet)
	if err != nil {
		return err
	}

	destBox, err := s.store.GetMailbox(context.Background(), s.user.Username, dest)
	if err != nil {
		return &imap.Error{Type: imap.StatusResponseTypeNo, Text: "Mailbox does not exist"}
	}

	sourceUIDs, destUIDs, err := s.store.Move(context.Background(), s.mailbox.ID, destBox.ID, uids)
	if err != nil {
		return err
	}

	if err := w.WriteCopyData(copyData(destBox.UIDValidity, sourceUIDs, destUIDs)); err != nil {
		return err
	}

	var seqs []uint32

	for _, uid := range sourceUIDs {
		if seq, ok := s.view.Seq(uid); ok {
			seqs = append(seqs, seq)
		}
	}

	for _, uid := range sourceUIDs {
		_, _ = s.view.Remove(uid)
		delete(s.flags, uid)
	}

	sort.Slice(seqs, func(i, j int) bool { return seqs[i] > seqs[j] })

	for _, seq := range seqs {
		if err := w.WriteExpunge(seq); err != nil {
			return err
		}
	}

	return nil
}

func (s *Session) Namespace() (*imap.NamespaceData, error) {
	return &imap.NamespaceData{
		Personal: []imap.NamespaceDescriptor{{Delim: defaultDelimiter}},
	}, nil
}

func (s *Session) resolve(numSet imap.NumSet) ([]uint32, error) {
	switch set := numSet.(type) {
	case imap.SeqSet:
		var uids []uint32

		for _, r := range set {
			start, stop := r.Start, r.Stop
			if start == 0 {
				start = 1
			}

			if stop == 0 {
				stop = s.view.Count()
			}

			if start > stop {
				start, stop = stop, start
			}

			for seq := start; seq <= stop; seq++ {
				if uid, ok := s.view.UIDAt(seq); ok {
					uids = append(uids, uid)
				}
			}
		}

		return uniqueUIDs(uids), nil
	case imap.UIDSet:
		var uids []uint32

		for _, r := range set {
			start, stop := uint32(r.Start), uint32(r.Stop)
			if start == 0 {
				start = 1
			}

			if stop == 0 {
				stop = s.view.HighestUID()
			}

			if start > stop {
				start, stop = stop, start
			}

			for uid := start; uid <= stop; uid++ {
				if _, ok := s.view.Seq(uid); ok {
					uids = append(uids, uid)
				}
			}
		}

		return uniqueUIDs(uids), nil
	default:
		return nil, fmt.Errorf("%w: %T", ErrUnsupportedNumSet, numSet)
	}
}

func uniqueUIDs(uids []uint32) []uint32 {
	seen := make(map[uint32]struct{}, len(uids))
	result := make([]uint32, 0, len(uids))

	for _, uid := range uids {
		if _, ok := seen[uid]; ok {
			continue
		}

		seen[uid] = struct{}{}
		result = append(result, uid)
	}

	return result
}

func toIMAPFlags(flags []string) []imap.Flag {
	result := make([]imap.Flag, 0, len(flags))
	for _, flag := range flags {
		result = append(result, imap.Flag(flag))
	}

	return result
}

func sameFlags(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}

	set := make(map[string]struct{}, len(a))
	for _, flag := range a {
		set[strings.ToLower(flag)] = struct{}{}
	}

	for _, flag := range b {
		if _, ok := set[strings.ToLower(flag)]; !ok {
			return false
		}
	}

	return true
}

func searchNeedsBody(criteria *imap.SearchCriteria) bool {
	if len(criteria.Body) > 0 || len(criteria.Text) > 0 || len(criteria.Header) > 0 {
		return true
	}

	for i := range criteria.Not {
		if searchNeedsBody(&criteria.Not[i]) {
			return true
		}
	}

	for i := range criteria.Or {
		if searchNeedsBody(&criteria.Or[i][0]) || searchNeedsBody(&criteria.Or[i][1]) {
			return true
		}
	}

	return false
}

func fetchNeedsBody(options *imap.FetchOptions) bool {
	if options.Envelope || options.BodyStructure != nil {
		return true
	}

	if len(options.BodySection) > 0 || len(options.BinarySection) > 0 || len(options.BinarySectionSize) > 0 {
		return true
	}

	return false
}

func copyData(uidValidity uint32, source, dest []uint32) *imap.CopyData {
	src := imap.UIDSet{}
	for _, uid := range source {
		src.AddNum(imap.UID(uid))
	}

	dst := imap.UIDSet{}
	for _, uid := range dest {
		dst.AddNum(imap.UID(uid))
	}

	return &imap.CopyData{UIDValidity: uidValidity, SourceUIDs: src, DestUIDs: dst}
}

func matchListPatterns(ref, name string, patterns []string) bool {
	if len(patterns) == 0 {
		return true
	}

	for _, pattern := range patterns {
		full := pattern
		if ref != "" && !strings.HasPrefix(pattern, string(defaultDelimiter)) {
			full = strings.TrimSuffix(ref, string(defaultDelimiter)) + string(defaultDelimiter) + pattern
		}

		if matchWildcard(full, name) {
			return true
		}
	}

	return false
}

func matchWildcard(pattern, name string) bool {
	var b strings.Builder
	b.WriteString("^")

	for _, r := range pattern {
		switch r {
		case '*':
			b.WriteString(".*")
		case '%':
			b.WriteString("[^" + string(defaultDelimiter) + "]*")
		default:
			if strings.ContainsRune(`.+()|[]{}^$\\`, r) {
				b.WriteByte('\\')
			}

			b.WriteRune(r)
		}
	}

	b.WriteString("$")

	ok, err := regexp.MatchString(b.String(), name)
	if err != nil {
		return false
	}

	return ok
}

func mailboxAttrs(box store.Mailbox, names map[string]struct{}) []imap.MailboxAttr {
	attrs := []imap.MailboxAttr{}

	prefix := box.Name + string(defaultDelimiter)
	hasChildren := false

	for name := range names {
		if strings.HasPrefix(name, prefix) {
			hasChildren = true
			break
		}
	}

	if hasChildren {
		attrs = append(attrs, imap.MailboxAttrHasChildren)
	} else {
		attrs = append(attrs, imap.MailboxAttrHasNoChildren)
	}

	if box.Subscribed {
		attrs = append(attrs, imap.MailboxAttrSubscribed)
	}

	if strings.EqualFold(box.Name, store.InboxName) {
		attrs = append(attrs, imap.MailboxAttr("\\Inbox"))
	}

	return attrs
}

var (
	_ imapserver.Session          = (*Session)(nil)
	_ imapserver.SessionMove      = (*Session)(nil)
	_ imapserver.SessionNamespace = (*Session)(nil)
)

// ErrUnsupportedNumSet is returned for an unknown IMAP number set type.
var ErrUnsupportedNumSet = errors.New("unsupported number set")
