package pgnotify

import (
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"uuid"
)

// ErrInvalidFrame is returned when a notification frame is malformed.
var ErrInvalidFrame = errors.New("invalid notification frame")

const (
	// MaxPayloadSize is the PostgreSQL payload limit. Payloads must be shorter.
	MaxPayloadSize = 8000

	frameFlagsOffset  = 0
	frameIndexOffset  = frameFlagsOffset + 1
	frameSenderOffset = frameIndexOffset + 4
	// FrameHeaderSize is the size of the frame header. It has one flags byte,
	// a 32-bit frame index, and a 16-byte sender id.
	FrameHeaderSize = frameSenderOffset + 16
	// FrameMoreFlag is set in the flags byte when more frames follow.
	FrameMoreFlag = 1 << 7

	// ChunkSize is the maximum number of payload bytes in one frame. The
	// base64 encoding of the frame header and payload must fit in MaxPayloadSize.
	ChunkSize = 5976
)

type frame struct {
	data   []byte
	sender uuid.UUID
	index  uint32
	more   bool
}

// encode splits a payload into frames. The frame index makes every frame
// unique, so PostgreSQL does not coalesce identical chunks.
func encode(payload []byte, sender uuid.UUID) []string {
	if len(payload) == 0 {
		return []string{encodeFrame(frame{sender: sender})}
	}

	frames := make([]string, 0, (len(payload)+ChunkSize-1)/ChunkSize)

	for index := uint32(0); len(payload) > 0; index++ {
		size := min(ChunkSize, len(payload))
		chunk := payload[:size]
		payload = payload[size:]

		frames = append(frames, encodeFrame(frame{
			data:   chunk,
			sender: sender,
			index:  index,
			more:   len(payload) > 0,
		}))
	}

	return frames
}

func encodeFrame(f frame) string {
	raw := make([]byte, FrameHeaderSize+len(f.data))

	if f.more {
		raw[frameFlagsOffset] = FrameMoreFlag
	}

	binary.BigEndian.PutUint32(raw[frameIndexOffset:frameSenderOffset], f.index)
	copy(raw[frameSenderOffset:FrameHeaderSize], f.sender[:])
	copy(raw[FrameHeaderSize:], f.data)

	return base64.StdEncoding.EncodeToString(raw)
}

func decodeFrame(raw string) (frame, error) {
	if len(raw) >= MaxPayloadSize {
		return frame{}, fmt.Errorf("%w: frame is too large", ErrInvalidFrame)
	}

	buf, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return frame{}, fmt.Errorf("%w: %w", ErrInvalidFrame, err)
	}

	if len(buf) < FrameHeaderSize {
		return frame{}, fmt.Errorf("%w: frame is too short", ErrInvalidFrame)
	}

	header := buf[frameFlagsOffset]
	if header&^FrameMoreFlag != 0 {
		return frame{}, fmt.Errorf("%w: unknown header bits", ErrInvalidFrame)
	}

	f := frame{
		data: buf[FrameHeaderSize:],
		index: uint32(buf[frameIndexOffset])<<24 |
			uint32(buf[frameIndexOffset+1])<<16 |
			uint32(buf[frameIndexOffset+2])<<8 |
			uint32(buf[frameIndexOffset+3]),
		more: header&FrameMoreFlag != 0,
	}
	copy(f.sender[:], buf[frameSenderOffset:FrameHeaderSize])

	return f, nil
}
