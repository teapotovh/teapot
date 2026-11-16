package imap

import (
	"testing"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/stretchr/testify/require"
)

const sampleMessage = "From: Alice <alice@example.org>\r\n" +
	"To: Bob <bob@example.net>\r\n" +
	"Subject: Meeting on Friday\r\n" +
	"Date: Fri, 14 Nov 2025 09:30:00 +0100\r\n" +
	"Message-Id: <one@example.org>\r\n" +
	"Content-Type: text/plain; charset=utf-8\r\n" +
	"\r\n" +
	"Hello Bob,\r\n" +
	"\r\n" +
	"Let's meet on Friday to discuss the IMAP server.\r\n" +
	"\r\n" +
	"Alice\r\n"

func sampleSearchMessage() SearchMessage {
	return SearchMessage{
		UID:          42,
		InternalDate: time.Date(2025, 11, 10, 15, 30, 0, 0, time.FixedZone("CET", 3600)),
		Size:         int64(len(sampleMessage)),
		Flags:        []string{"\\Seen", "\\Flagged"},
		Body:         []byte(sampleMessage),
		ModSeq:       10,
	}
}

func TestMatchSearch(t *testing.T) {
	t.Parallel()

	msg := sampleSearchMessage()
	size := msg.Size

	noCriteria := (*imap.SearchCriteria)(nil)

	tests := []struct {
		name     string
		seq      uint32
		msg      SearchMessage
		criteria *imap.SearchCriteria
		want     bool
	}{
		{
			name:     "nil criteria",
			seq:      1,
			criteria: noCriteria,
			want:     true,
		},
		{
			name:     "empty criteria",
			seq:      1,
			criteria: &imap.SearchCriteria{},
			want:     true,
		},
		{
			name:     "sequence number match",
			seq:      3,
			criteria: &imap.SearchCriteria{SeqNum: []imap.SeqSet{imap.SeqSetNum(3)}},
			want:     true,
		},
		{
			name:     "sequence number mismatch",
			seq:      3,
			criteria: &imap.SearchCriteria{SeqNum: []imap.SeqSet{imap.SeqSetNum(4)}},
			want:     false,
		},
		{
			name:     "uid match",
			seq:      1,
			criteria: &imap.SearchCriteria{UID: []imap.UIDSet{imap.UIDSetNum(42)}},
			want:     true,
		},
		{
			name:     "uid mismatch",
			seq:      1,
			criteria: &imap.SearchCriteria{UID: []imap.UIDSet{imap.UIDSetNum(43)}},
			want:     false,
		},
		{
			name: "since date match",
			seq:  1,
			criteria: &imap.SearchCriteria{
				Since: time.Date(2025, 11, 10, 0, 0, 0, 0, time.UTC),
			},
			want: true,
		},
		{
			name: "since date mismatch",
			seq:  1,
			criteria: &imap.SearchCriteria{
				Since: time.Date(2025, 11, 11, 0, 0, 0, 0, time.UTC),
			},
			want: false,
		},
		{
			name: "before date match",
			seq:  1,
			criteria: &imap.SearchCriteria{
				Before: time.Date(2025, 11, 11, 0, 0, 0, 0, time.UTC),
			},
			want: true,
		},
		{
			name: "before date mismatch",
			seq:  1,
			criteria: &imap.SearchCriteria{
				Before: time.Date(2025, 11, 10, 0, 0, 0, 0, time.UTC),
			},
			want: false,
		},
		{
			name: "sent since match",
			seq:  1,
			criteria: &imap.SearchCriteria{
				SentSince: time.Date(2025, 11, 14, 0, 0, 0, 0, time.UTC),
			},
			want: true,
		},
		{
			name: "sent since mismatch",
			seq:  1,
			criteria: &imap.SearchCriteria{
				SentSince: time.Date(2025, 11, 15, 0, 0, 0, 0, time.UTC),
			},
			want: false,
		},
		{
			name: "sent before match",
			seq:  1,
			criteria: &imap.SearchCriteria{
				SentBefore: time.Date(2025, 11, 15, 0, 0, 0, 0, time.UTC),
			},
			want: true,
		},
		{
			name: "sent before mismatch",
			seq:  1,
			criteria: &imap.SearchCriteria{
				SentBefore: time.Date(2025, 11, 14, 0, 0, 0, 0, time.UTC),
			},
			want: false,
		},
		{
			name: "header match",
			seq:  1,
			criteria: &imap.SearchCriteria{
				Header: []imap.SearchCriteriaHeaderField{{Key: "Subject", Value: "friday"}},
			},
			want: true,
		},
		{
			name: "header key is case insensitive",
			seq:  1,
			criteria: &imap.SearchCriteria{
				Header: []imap.SearchCriteriaHeaderField{{Key: "subject", Value: "MEETING"}},
			},
			want: true,
		},
		{
			name: "header value mismatch",
			seq:  1,
			criteria: &imap.SearchCriteria{
				Header: []imap.SearchCriteriaHeaderField{{Key: "Subject", Value: "Monday"}},
			},
			want: false,
		},
		{
			name: "header presence",
			seq:  1,
			criteria: &imap.SearchCriteria{
				Header: []imap.SearchCriteriaHeaderField{{Key: "Message-Id"}},
			},
			want: true,
		},
		{
			name: "header absence",
			seq:  1,
			criteria: &imap.SearchCriteria{
				Header: []imap.SearchCriteriaHeaderField{{Key: "X-Missing"}},
			},
			want: false,
		},
		{
			name: "body match",
			seq:  1,
			criteria: &imap.SearchCriteria{
				Body: []string{"discuss the IMAP server"},
			},
			want: true,
		},
		{
			name: "body is case insensitive",
			seq:  1,
			criteria: &imap.SearchCriteria{
				Body: []string{"IMAP SERVER"},
			},
			want: true,
		},
		{
			name: "body does not include header",
			seq:  1,
			criteria: &imap.SearchCriteria{
				Body: []string{"Meeting on Friday"},
			},
			want: false,
		},
		{
			name: "text includes header",
			seq:  1,
			criteria: &imap.SearchCriteria{
				Text: []string{"Meeting on Friday"},
			},
			want: true,
		},
		{
			name: "text includes body",
			seq:  1,
			criteria: &imap.SearchCriteria{
				Text: []string{"discuss the IMAP server"},
			},
			want: true,
		},
		{
			name: "text mismatch",
			seq:  1,
			criteria: &imap.SearchCriteria{
				Text: []string{"POP3"},
			},
			want: false,
		},
		{
			name: "flag match",
			seq:  1,
			criteria: &imap.SearchCriteria{
				Flag: []imap.Flag{imap.FlagSeen},
			},
			want: true,
		},
		{
			name: "flag mismatch",
			seq:  1,
			criteria: &imap.SearchCriteria{
				Flag: []imap.Flag{imap.FlagDeleted},
			},
			want: false,
		},
		{
			name: "not flag match",
			seq:  1,
			criteria: &imap.SearchCriteria{
				NotFlag: []imap.Flag{imap.FlagDeleted},
			},
			want: true,
		},
		{
			name: "not flag mismatch",
			seq:  1,
			criteria: &imap.SearchCriteria{
				NotFlag: []imap.Flag{imap.FlagSeen},
			},
			want: false,
		},
		{
			name: "flag comparison is case insensitive",
			seq:  1,
			msg: SearchMessage{
				UID:          42,
				InternalDate: msg.InternalDate,
				Size:         msg.Size,
				Flags:        []string{"\\seen"},
				Body:         msg.Body,
				ModSeq:       msg.ModSeq,
			},
			criteria: &imap.SearchCriteria{
				Flag: []imap.Flag{imap.FlagSeen},
			},
			want: true,
		},
		{
			name: "larger match",
			seq:  1,
			criteria: &imap.SearchCriteria{
				Larger: size - 1,
			},
			want: true,
		},
		{
			name: "larger mismatch",
			seq:  1,
			criteria: &imap.SearchCriteria{
				Larger: size,
			},
			want: false,
		},
		{
			name: "smaller match",
			seq:  1,
			criteria: &imap.SearchCriteria{
				Smaller: size + 1,
			},
			want: true,
		},
		{
			name: "smaller mismatch",
			seq:  1,
			criteria: &imap.SearchCriteria{
				Smaller: size,
			},
			want: false,
		},
		{
			name: "not match",
			seq:  1,
			criteria: &imap.SearchCriteria{
				Not: []imap.SearchCriteria{{Flag: []imap.Flag{imap.FlagDeleted}}},
			},
			want: true,
		},
		{
			name: "not mismatch",
			seq:  1,
			criteria: &imap.SearchCriteria{
				Not: []imap.SearchCriteria{{Flag: []imap.Flag{imap.FlagSeen}}},
			},
			want: false,
		},
		{
			name: "or first branch",
			seq:  1,
			criteria: &imap.SearchCriteria{
				Or: [][2]imap.SearchCriteria{{
					{Flag: []imap.Flag{imap.FlagDeleted}},
					{Body: []string{"imap server"}},
				}},
			},
			want: true,
		},
		{
			name: "or second branch",
			seq:  1,
			criteria: &imap.SearchCriteria{
				Or: [][2]imap.SearchCriteria{{
					{Body: []string{"imap server"}},
					{Flag: []imap.Flag{imap.FlagDeleted}},
				}},
			},
			want: true,
		},
		{
			name: "or mismatch",
			seq:  1,
			criteria: &imap.SearchCriteria{
				Or: [][2]imap.SearchCriteria{{
					{Flag: []imap.Flag{imap.FlagDeleted}},
					{Body: []string{"POP3"}},
				}},
			},
			want: false,
		},
		{
			name: "mod seq match",
			seq:  1,
			criteria: &imap.SearchCriteria{
				ModSeq: &imap.SearchCriteriaModSeq{ModSeq: 10},
			},
			want: true,
		},
		{
			name: "mod seq mismatch",
			seq:  1,
			criteria: &imap.SearchCriteria{
				ModSeq: &imap.SearchCriteriaModSeq{ModSeq: 11},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			message := tt.msg
			if message.UID == 0 {
				message = msg
			}

			got := MatchSearch(tt.seq, message, tt.criteria)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestMatchSearchMultipart(t *testing.T) {
	t.Parallel()

	const multipartMessage = "From: Alice <alice@example.org>\r\n" +
		"Subject: Multipart\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: multipart/mixed; boundary=\"BOUND\"\r\n" +
		"\r\n" +
		"--BOUND\r\n" +
		"Content-Type: text/plain\r\n" +
		"\r\n" +
		"first part has a needle\r\n" +
		"--BOUND\r\n" +
		"Content-Type: application/octet-stream\r\n" +
		"\r\n" +
		"binary needle\r\n" +
		"--BOUND--\r\n"

	msg := SearchMessage{
		UID:          1,
		InternalDate: time.Date(2025, 11, 10, 0, 0, 0, 0, time.UTC),
		Size:         int64(len(multipartMessage)),
		Body:         []byte(multipartMessage),
	}

	require.True(t, MatchSearch(1, msg, &imap.SearchCriteria{
		Body: []string{"has a needle"},
	}))

	require.False(t, MatchSearch(1, msg, &imap.SearchCriteria{
		Body: []string{"binary needle"},
	}))

	require.True(t, MatchSearch(1, msg, &imap.SearchCriteria{
		Text: []string{"first part"},
	}))
}
