package store

import (
	"fmt"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	filespb "github.com/teapotovh/teapot/proto/files"
)

const (
	// RootNodeID is the node id of the root directory.
	RootNodeID uint64 = 1

	// defaultChannel is the PostgreSQL notification channel.
	defaultChannel = "files"

	// PageSize is the number of inodes returned by one list page.
	PageSize = 256

	dirKind  int16 = 0
	fileKind int16 = 1

	maxDepth = 256
)

// Inode is the stored state of a file or a directory.
type Inode struct {
	UID      int64
	NodeID   uint64
	ParentID uint64
	Name     string
	Kind     Kind
	Size     int64
	ModTime  time.Time
	Hash     Hash
	S3Key    string
	Revision uint64
	Deleted  bool
}

// Event is a change to one inode.
type Event struct {
	Inode Inode
}

// SnapshotCursor is a position in the snapshot order.
type SnapshotCursor struct {
	ParentID uint64
	Name     string
}

// Proto returns the wire form of the inode.
func (i Inode) Proto() *filespb.Inode {
	message := &filespb.Inode{
		Identifier: &filespb.InodeIdentifier{
			Uid:    i.UID,
			NodeId: i.NodeID,
		},
		Revision:   i.Revision,
		ParentId:   i.ParentID,
		Name:       i.Name,
		ModifiedAt: timestamppb.New(i.ModTime),
	}

	if i.Kind == KindFile {
		message.Kind = &filespb.Inode_File{File: &filespb.FileKind{
			Ref:  i.S3Key,
			Hash: []byte(i.Hash),
			Size: i.Size,
		}}
	} else {
		message.Kind = &filespb.Inode_Directory{Directory: &filespb.DirectoryKind{}}
	}

	return message
}

// Proto returns the wire form of the event.
func (e Event) Proto() *filespb.Event {
	if e.Inode.Deleted {
		return &filespb.Event{Event: &filespb.Event_Delete{Delete: &filespb.Delete{
			Identifier: &filespb.InodeIdentifier{
				Uid:    e.Inode.UID,
				NodeId: e.Inode.NodeID,
			},
			Revision: e.Inode.Revision,
		}}}
	}

	return &filespb.Event{Event: &filespb.Event_Update{Update: &filespb.Update{
		Inode: e.Inode.Proto(),
	}}}
}

func fromProtoInode(message *filespb.Inode) (Inode, error) {
	if message == nil {
		return Inode{}, fmt.Errorf("%w: missing inode", ErrCorruptData)
	}

	inode := Inode{
		UID:      message.GetIdentifier().GetUid(),
		NodeID:   message.GetIdentifier().GetNodeId(),
		ParentID: message.GetParentId(),
		Name:     message.GetName(),
		Kind:     KindDirectory,
		Revision: message.GetRevision(),
	}

	if modified := message.GetModifiedAt(); modified != nil {
		inode.ModTime = modified.AsTime()
	}

	if file := message.GetFile(); file != nil {
		inode.Kind = KindFile
		inode.S3Key = file.GetRef()
		inode.Hash = Hash(file.GetHash())
		inode.Size = file.GetSize()
	}

	return inode, nil
}

func encodeInode(inode Inode) ([]byte, error) {
	return inode.Proto().MarshalVT()
}

func decodeInode(data []byte) (Inode, error) {
	var message filespb.Inode
	if err := message.UnmarshalVT(data); err != nil {
		return Inode{}, fmt.Errorf("%w: error while decoding inode: %w", ErrCorruptData, err)
	}

	return fromProtoInode(&message)
}

func encodeEvent(event Event) ([]byte, error) {
	return event.Proto().MarshalVT()
}

func decodeEvent(data []byte) (Event, error) {
	var message filespb.Event
	if err := message.UnmarshalVT(data); err != nil {
		return Event{}, fmt.Errorf("error while decoding event: %w", err)
	}

	switch change := message.GetEvent().(type) {
	case *filespb.Event_Update:
		inode, err := fromProtoInode(change.Update.GetInode())
		if err != nil {
			return Event{}, err
		}

		return Event{Inode: inode}, nil
	case *filespb.Event_Delete:
		identifier := change.Delete.GetIdentifier()

		return Event{Inode: Inode{
			UID:      identifier.GetUid(),
			NodeID:   identifier.GetNodeId(),
			Revision: change.Delete.GetRevision(),
			Deleted:  true,
		}}, nil
	default:
		return Event{}, fmt.Errorf("%w: missing event", ErrCorruptData)
	}
}

func inodeKind(kind Kind) (int16, error) {
	switch kind {
	case KindDirectory:
		return dirKind, nil
	case KindFile:
		return fileKind, nil
	default:
		return 0, ErrInvalidKind
	}
}
