package pgnotify

import (
	"bytes"
	"errors"
	"testing"

	"github.com/google/uuid"
)

var (
	testSender  = uuid.MustParse("11111111-1111-1111-1111-111111111111")
	otherSender = uuid.MustParse("22222222-2222-2222-2222-222222222222")
)

func TestEncodeDecodeRoundTrip(t *testing.T) {
	t.Parallel()

	payloads := [][]byte{
		nil,
		[]byte("hello"),
		bytes.Repeat([]byte("a"), ChunkSize),
		bytes.Repeat([]byte("b"), ChunkSize+1),
		bytes.Repeat([]byte("c"), 2*ChunkSize+17),
		bytes.Repeat([]byte{0, 1, 2, 255}, 50000),
		make([]byte, 3*ChunkSize),
	}

	for _, payload := range payloads {
		frames := encode(payload, testSender)

		var decoded []byte

		seen := map[string]struct{}{}

		for i, raw := range frames {
			if len(raw) > MaxPayloadSize {
				t.Fatalf("frame %d is %d bytes, expected at most %d", i, len(raw), MaxPayloadSize)
			}

			if _, ok := seen[raw]; ok {
				t.Fatalf("frame %d is not unique, PostgreSQL coalesces identical frames", i)
			}

			seen[raw] = struct{}{}

			frame, err := decodeFrame(raw)
			if err != nil {
				t.Fatalf("error while decoding frame %d: %v", i, err)
			}

			if frame.sender != testSender {
				t.Fatalf("frame %d has sender %v, expected %v", i, frame.sender, testSender)
			}

			if frame.index != uint32(i) {
				t.Fatalf("frame %d has index %d, expected %d", i, frame.index, i)
			}

			if frame.more != (i < len(frames)-1) {
				t.Fatalf("frame %d has more=%t, expected %t", i, frame.more, i < len(frames)-1)
			}

			decoded = append(decoded, frame.data...)
		}

		if !bytes.Equal(decoded, payload) {
			t.Fatalf(
				"decoded payload does not match the original: got %d bytes, expected %d",
				len(decoded),
				len(payload),
			)
		}
	}
}

func TestDecodeFrameInvalid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		frame string
	}{
		{name: "empty", frame: ""},
		{name: "short header", frame: "\x00\x00"},
		{name: "unknown header bits", frame: "\x01\x00\x00\x00\x00" + string(make([]byte, 16)) + "AAAA"},
		{name: "invalid base64", frame: "\x00\x00\x00\x00\x00" + string(make([]byte, 16)) + "!!!!"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			_, err := decodeFrame(test.frame)
			if !errors.Is(err, ErrInvalidFrame) {
				t.Fatalf("expected ErrInvalidFrame, got %v", err)
			}
		})
	}
}
