package grpc

import (
	"context"
	"errors"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	filespb "github.com/teapotovh/teapot/proto/files"
	"github.com/teapotovh/teapot/service/files/store"
)

// Store is the source of snapshot pages and watch events.
type Store interface {
	// Snapshot sends pages of inodes after the requested revision.
	Snapshot(ctx context.Context, after uint64, send func(revision uint64, inodes []store.Inode) error) error
	// Watch sends events after the requested revision.
	Watch(ctx context.Context, after uint64, send func(event store.Event) error) error
}

// Server implements the files gRPC service.
type Server struct {
	store Store
}

// NewServer creates a files gRPC server.
func NewServer(store Store) *Server {
	return &Server{store: store}
}

// Snapshot implements filespb.FilesServer.
func (s *Server) Snapshot(_ *filespb.SnapshotRequest, stream filespb.Files_SnapshotServer) error {
	return toStatusError(s.store.Snapshot(
		stream.Context(),
		0,
		func(revision uint64, inodes []store.Inode) error {
			page := &filespb.SnapshotPage{
				Revision: revision,
				Inodes:   make([]*filespb.Inode, 0, len(inodes)),
			}

			for _, inode := range inodes {
				page.Inodes = append(page.Inodes, inode.Proto())
			}

			return stream.Send(page)
		},
	))
}

// Watch implements filespb.FilesServer.
func (s *Server) Watch(request *filespb.WatchRequest, stream filespb.Files_WatchServer) error {
	return toStatusError(s.store.Watch(
		stream.Context(),
		request.GetAfterRevision(),
		func(event store.Event) error {
			return stream.Send(&filespb.WatchEvent{Event: event.Proto()})
		},
	))
}

// Register registers the server with a gRPC server.
func (s *Server) Register(server *grpc.Server) {
	filespb.RegisterFilesServer(server, s)
}

func toStatusError(err error) error {
	if err == nil {
		return nil
	}

	if errors.Is(err, store.ErrRevisionTooOld) {
		return status.Error(codes.OutOfRange, store.ErrRevisionTooOld.Error())
	}

	if _, ok := status.FromError(err); ok {
		return err
	}

	return status.Error(codes.Internal, err.Error())
}

var _ filespb.FilesServer = (*Server)(nil)
var _ Store = (*store.Store)(nil)
