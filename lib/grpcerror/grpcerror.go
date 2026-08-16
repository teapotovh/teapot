package grpcerror

import (
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type grpcError struct {
	code codes.Code
	err  error
}

func Wrap(code codes.Code, err error) error {
	return &grpcError{code: code, err: err}
}

func (e *grpcError) Error() string { return e.err.Error() }

func (e *grpcError) Unwrap() error { return e.err }

func (e *grpcError) GRPCStatus() *status.Status { return status.New(e.code, e.err.Error()) }

// grpcstatus is lifted from the grpc's status package.
type grpcstatus interface{ GRPCStatus() *status.Status }

// Ensure *grpcError implements grpcstatus
var _ grpcstatus = (*grpcError)(nil)
