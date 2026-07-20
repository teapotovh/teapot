package ldapsrv

import (
	"context"

	"github.com/google/uuid"
)

type ContextKey string

const (
	ContextKeyRequestID ContextKey = "requestid"
	ContextKeyAddr      ContextKey = "addr"
)

func RequestID(ctx context.Context) uuid.UUID {
	if v := ctx.Value(ContextKeyRequestID); v != nil {
		return v.(uuid.UUID)
	}

	return uuid.New()
}
