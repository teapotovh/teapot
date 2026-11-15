package imap

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"time"

	"github.com/emersion/go-imap/v2"
	gomessage "github.com/emersion/go-message"
	"github.com/emersion/go-message/mail"
)

// SearchMessage is the message data used by MatchSearch.
type SearchMessage struct {
	UID          uint32
	InternalDate time.Time
	Size         int64
	Flags        []string
	Body         []byte
	ModSeq       uint64
}

// MatchSearch reports whether a message matches the given IMAP SEARCH
// criteria. The seq argument is the message sequence number.
//
//nolint:gocyclo
func MatchSearch(seq uint32, msg SearchMessage, criteria *imap.SearchCriteria) bool {
	if criteria == nil {
		return true
	}

	for _, seqSet := range criteria.SeqNum {
		if seq == 0 || !seqSet.Contains(seq) {
			return false
		}
	}

	for _, uidSet := range criteria.UID {
		if !uidSet.Contains(imap.UID(msg.UID)) {
			return false
		}
	}

	if !matchDate(msg.InternalDate, criteria.Since, criteria.Before) {
		return false
	}

	flags := make(map[string]struct{}, len(msg.Flags))
	for _, flag := range msg.Flags {
		flags[strings.ToLower(flag)] = struct{}{}
	}

	for _, flag := range criteria.Flag {
		if _, ok := flags[strings.ToLower(string(flag))]; !ok {
			return false
		}
	}

	for _, flag := range criteria.NotFlag {
		if _, ok := flags[strings.ToLower(string(flag))]; ok {
			return false
		}
	}

	if criteria.Larger != 0 && msg.Size <= criteria.Larger {
		return false
	}

	if criteria.Smaller != 0 && msg.Size >= criteria.Smaller {
		return false
	}

	if criteria.ModSeq != nil && msg.ModSeq < criteria.ModSeq.ModSeq {
		return false
	}

	entity := readEntity(msg.Body)
	header := mail.Header{Header: entity.Header}

	for _, field := range criteria.Header {
		if !matchHeaderFields(header.FieldsByKey(field.Key), field.Value) {
			return false
		}
	}

	if !criteria.SentSince.IsZero() || !criteria.SentBefore.IsZero() {
		t, err := header.Date()
		if err != nil {
			return false
		}

		if !matchDate(t, criteria.SentSince, criteria.SentBefore) {
			return false
		}
	}

	for _, text := range criteria.Text {
		if !matchEntity(entity, text, true) {
			return false
		}
	}

	for _, body := range criteria.Body {
		if !matchEntity(entity, body, false) {
			return false
		}
	}

	for i := range criteria.Not {
		if MatchSearch(seq, msg, &criteria.Not[i]) {
			return false
		}
	}

	for i := range criteria.Or {
		if !MatchSearch(seq, msg, &criteria.Or[i][0]) &&
			!MatchSearch(seq, msg, &criteria.Or[i][1]) {
			return false
		}
	}

	return true
}

func readEntity(body []byte) *gomessage.Entity {
	entity, _ := gomessage.Read(bytes.NewReader(body))
	if entity == nil {
		entity, _ = gomessage.New(gomessage.Header{}, bytes.NewReader(nil))
	}

	return entity
}

// matchDate compares dates without time zone information, as required by
// RFC 3501.
func matchDate(t, since, before time.Time) bool {
	t = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)

	if !since.IsZero() && t.Before(since) {
		return false
	}

	if !before.IsZero() && !t.Before(before) {
		return false
	}

	return true
}

func matchHeaderFields(fields gomessage.HeaderFields, pattern string) bool {
	if pattern == "" {
		return fields.Len() > 0
	}

	pattern = strings.ToLower(pattern)

	for fields.Next() {
		value, _ := fields.Text()
		if strings.Contains(strings.ToLower(value), pattern) {
			return true
		}
	}

	return false
}

func matchEntity(entity *gomessage.Entity, pattern string, includeHeader bool) bool {
	if pattern == "" {
		return true
	}

	if includeHeader && matchHeaderFields(entity.Header.Fields(), pattern) {
		return true
	}

	if reader := entity.MultipartReader(); reader != nil {
		for {
			part, err := reader.NextPart()
			if errors.Is(err, io.EOF) {
				break
			}

			if err != nil {
				return false
			}

			if matchEntity(part, pattern, includeHeader) {
				return true
			}
		}

		return false
	}

	contentType, _, err := entity.Header.ContentType()
	if err != nil {
		return false
	}

	if !strings.HasPrefix(contentType, "text/") &&
		!strings.HasPrefix(contentType, "message/") {
		return false
	}

	body, err := io.ReadAll(entity.Body)
	if err != nil {
		return false
	}

	return bytes.Contains(bytes.ToLower(body), bytes.ToLower([]byte(pattern)))
}
