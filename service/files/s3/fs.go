// Package s3 contains an example S3 file system.
package s3

import (
	"github.com/hack-pad/hackpadfs"
	"github.com/hack-pad/hackpadfs/keyvalue"
)

// FS is an S3-based file system, storing files and metadata in an object storage bucket.
type FS struct {
	*keyvalue.FS
	store *store
}

// Options provides configuration options for a new FS.
type Options struct {
	Endpoint        string
	BucketName      string
	AccessKeyID     string
	SecretAccessKey string
	Insecure        bool
}

// NewFS returns a new FS.
func NewFS(options Options) (*FS, error) {
	store, err := newStore(options)
	if err != nil {
		return nil, err
	}
	kv, err := keyvalue.NewFS(store)
	return &FS{
		FS:    kv,
		store: store,
	}, err
}

// Ensure FS implements hackpadfs.FS
var _ hackpadfs.FS = FS{}
