package grpc

import (
	"errors"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/teapotovh/teapot/lib/grpcsrv"
	filespb "github.com/teapotovh/teapot/proto/files"
	filesPkg "github.com/teapotovh/teapot/service/files"
	"github.com/teapotovh/teapot/service/files/store"
)

// Files provides the store used by the files gRPC service.
type Files interface {
	// Store returns the file store.
	Store() *store.Store
}

// Server implements the files gRPC service.
type Server struct {
	store *store.Store
}

// NewServer creates a files gRPC server.
func NewServer(files Files) *Server {
	return &Server{store: files.Store()}
}

// Snapshot implements filespb.FilesServer.
func (s *Server) Snapshot(_ *filespb.SnapshotRequest, stream filespb.Files_SnapshotServer) error {
	return toStatusError(s.store.Snapshot(
		stream.Context(),
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

// Register implements grpcsrv.GRPCService.
func (s *Server) Register(server *grpc.Server) {
	filespb.RegisterFilesServer(server, s)
}

func toStatusError(err error) error {
	if err == nil {
		return nil
	}

	if errors.Is(err, store.ErrRevisionTooOld) {
		return status.Error(codes.OutOfRange, err.Error())
	}

	if _, ok := status.FromError(err); ok {
		return err
	}

	return status.Error(codes.Internal, err.Error())
}

// Ensure *Server implements filespb.FilesServer.
var _ filespb.FilesServer = (*Server)(nil)

// Ensure *Server implements grpcsrv.GRPCService.
var _ grpcsrv.GRPCService = (*Server)(nil)

// Ensure *filesPkg.Files implements Files.
var _ Files = (*filesPkg.Files)(nil)
