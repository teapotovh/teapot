package store

import (
	"context"
	"errors"
)

// ErrRevisionTooOld reports that the watch history is no longer available.
var ErrRevisionTooOld = errors.New("revision too old")

// Snapshot sends all live inodes from the cache.
func (s *Store) Snapshot(_ context.Context, send func(revision uint64, inodes []Inode) error) error {
	revision := s.revision.Load()
	batch := make([]Inode, 0, PageSize)
	sent := false

	flush := func() error {
		if len(batch) == 0 {
			return nil
		}

		if err := send(revision, batch); err != nil {
			return err
		}

		sent = true
		batch = nil

		return nil
	}

	for inode, err := range s.cache.all() {
		if err != nil {
			return err
		}

		batch = append(batch, inode)

		if len(batch) < PageSize {
			continue
		}

		if err := flush(); err != nil {
			return err
		}
	}

	if err := flush(); err != nil {
		return err
	}

	if !sent {
		return send(revision, nil)
	}

	return nil
}

// Watch sends live events after the requested revision.
func (s *Store) Watch(ctx context.Context, after uint64, send func(Event) error) error {
	subscriber := s.events.Subscribe()
	defer subscriber.Unsubscribe()

	if after != s.revision.Load() {
		return ErrRevisionTooOld
	}

	for event := range subscriber.Iter(ctx) {
		if event.Inode.Revision <= after {
			continue
		}

		if event.Inode.Revision > after+1 {
			return ErrRevisionTooOld
		}

		if err := send(event); err != nil {
			return err
		}

		after = event.Inode.Revision
	}

	return nil
}
