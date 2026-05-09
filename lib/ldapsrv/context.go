package ldapsrv

type ContextKey string

const (
	ContextKeyConnectionID ContextKey = "cid"
	ContextKeyUser         ContextKey = "user"
)
