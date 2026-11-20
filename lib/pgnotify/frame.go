package pgnotify

import (
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/google/uuid"
)

// ErrInvalidFrame is returned when a notification frame is malformed.
var ErrInvalidFrame = errors.New("invalid notification frame")

const (
	// MaxPayloadSize is the maximum size of a PostgreSQL notification payload.
	MaxPayloadSize = 8000

	frameFlagsOffset  = 0
	frameIndexOffset  = frameFlagsOffset + 1
	frameSenderOffset = frameIndexOffset + 4
	// FrameHeaderSize is the size of the frame header. It has one flags byte,
	// a 32-bit frame index, and a 16-byte sender id.
	FrameHeaderSize = frameSenderOffset + 16
	// FrameMoreFlag is set in the flags byte when more frames follow.
	FrameMoreFlag = 1 << 7

	// ChunkSize is the maximum number of raw bytes in one frame. Base64
	// encoding and the frame header must fit in MaxPayloadSize.
	ChunkSize = 5982
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
	raw := make([]byte, FrameHeaderSize+base64.StdEncoding.EncodedLen(len(f.data)))

	if f.more {
		raw[frameFlagsOffset] = FrameMoreFlag
	}

	binary.BigEndian.PutUint32(raw[frameIndexOffset:frameSenderOffset], f.index)
	copy(raw[frameSenderOffset:FrameHeaderSize], f.sender[:])
	base64.StdEncoding.Encode(raw[FrameHeaderSize:], f.data)

	return string(raw)
}

func decodeFrame(raw string) (frame, error) {
	if len(raw) < FrameHeaderSize {
		return frame{}, fmt.Errorf("%w: frame is too short", ErrInvalidFrame)
	}

	if len(raw) > MaxPayloadSize {
		return frame{}, fmt.Errorf("%w: frame is too large", ErrInvalidFrame)
	}

	header := raw[frameFlagsOffset]
	if header&^FrameMoreFlag != 0 {
		return frame{}, fmt.Errorf("%w: unknown header bits", ErrInvalidFrame)
	}

	data, err := base64.StdEncoding.DecodeString(raw[FrameHeaderSize:])
	if err != nil {
		return frame{}, fmt.Errorf("%w: %w", ErrInvalidFrame, err)
	}

	f := frame{
		data: data,
		index: uint32(raw[frameIndexOffset])<<24 |
			uint32(raw[frameIndexOffset+1])<<16 |
			uint32(raw[frameIndexOffset+2])<<8 |
			uint32(raw[frameIndexOffset+3]),
		more: header&FrameMoreFlag != 0,
	}
	copy(f.sender[:], raw[frameSenderOffset:FrameHeaderSize])

	return f, nil
}
