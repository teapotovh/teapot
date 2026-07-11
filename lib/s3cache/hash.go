package s3cache

import (
	"crypto/md5"
	"encoding/hex"
)

// Hash is an opaque content-version identifier, match S3's ETag.
type Hash string

const ZeroHash = Hash("")

// String returns the hash as a plain string.
func (h Hash) String() string {
	return string(h)
}

// IsZero reports whether h is ZeroHash.
func (h Hash) IsZero() bool {
	return h == ""
}

// HashBytes computes an MD5-hex Hash of data. This is the same format S3
// uses as the ETag for ordinary PUTs.
func HashBytes(data []byte) Hash {
	sum := md5.Sum(data)
	return Hash(hex.EncodeToString(sum[:]))
}
