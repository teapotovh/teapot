package ldapserver

type ContextKey string

const (
	ContextKeyConnectionID ContextKey = "cid"
	ContextKeyUser         ContextKey = "user"
)
