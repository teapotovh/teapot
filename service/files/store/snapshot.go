package store

import (
	"context"
	"errors"
)

// ErrRevisionTooOld reports that the watch history is no longer available.
var ErrRevisionTooOld = errors.New("revision too old")

// Snapshot sends pages of inodes after the requested revision.
func (s *Store) Snapshot(ctx context.Context, after uint64, send func(revision uint64, inodes []Inode) error) error {
	if err := s.checkRevision(ctx, after); err != nil {
		return err
	}

	tx, err := s.db.BeginSnapshot(ctx)
	if err != nil {
		return err
	}

	defer func() { _ = tx.Rollback(ctx) }()

	revision, err := tx.CurrentRevision(ctx)
	if err != nil {
		return err
	}

	cursor := Cursor{}
	sent := false

	for {
		inodes, next, more, err := tx.SnapshotPage(ctx, after, cursor, PageSize)
		if err != nil {
			return err
		}

		if len(inodes) > 0 {
			if err := send(revision, inodes); err != nil {
				return err
			}

			sent = true
		}

		if !more || len(inodes) == 0 {
			break
		}

		cursor = next
	}

	if !sent {
		if err := send(revision, nil); err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}

// Watch sends events after the requested revision.
func (s *Store) Watch(ctx context.Context, after uint64, send func(Event) error) error {
	if err := s.checkRevision(ctx, after); err != nil {
		return err
	}

	subscriber := s.events.Subscribe()
	defer subscriber.Unsubscribe()

	last, err := s.sendSince(ctx, after, send)
	if err != nil {
		return err
	}

	for event := range subscriber.Iter(ctx) {
		if event.Inode.Revision <= last {
			continue
		}

		if event.Inode.Revision > last+1 {
			last, err = s.sendSince(ctx, last, send)
			if err != nil {
				return err
			}

			continue
		}

		if err := send(event); err != nil {
			return err
		}

		last = event.Inode.Revision
	}

	return nil
}

func (s *Store) sendSince(ctx context.Context, after uint64, send func(Event) error) (uint64, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return after, err
	}

	defer func() { _ = tx.Rollback(ctx) }()

	inodes, err := tx.ChangedSince(ctx, after)
	if err != nil {
		return after, err
	}

	if err := tx.Commit(ctx); err != nil {
		return after, err
	}

	last := after
	for _, inode := range inodes {
		if inode.Revision < last {
			continue
		}

		if err := send(Event{Inode: inode}); err != nil {
			return last, err
		}

		if inode.Revision > last {
			last = inode.Revision
		}
	}

	return last, nil
}

func (s *Store) checkRevision(ctx context.Context, after uint64) error {
	if after == 0 {
		return nil
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}

	defer func() { _ = tx.Rollback(ctx) }()

	oldest, ok, err := tx.OldestTombstone(ctx)
	if err != nil {
		return err
	}

	if ok && after < oldest {
		return ErrRevisionTooOld
	}

	return nil
}
