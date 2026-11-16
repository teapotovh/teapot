package store

import (
	"crypto/rand"
	"encoding/binary"
)

func nowNano() int64 {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return 1
	}

	return int64(binary.LittleEndian.Uint64(buf[:]) & 0x7fffffffffffffff)
}
