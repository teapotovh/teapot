package ldapsrv

import "context"

type ContextKey string

const (
	ContextKeyRequestID ContextKey = "requestid"
	ContextKeyAddr      ContextKey = "addr"
	ContextKeyUser      ContextKey = "user"
)

func RequestID(ctx context.Context) string {
	if v := ctx.Value(ContextKeyRequestID); v != nil {
		return v.(string)
	}

	return ""
}
