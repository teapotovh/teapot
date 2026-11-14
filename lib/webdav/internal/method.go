package internal

// WebDAV and CalDAV method names that are not in net/http.
const (
	MethodACL        = "ACL"
	MethodCopy       = "COPY"
	MethodLock       = "LOCK"
	MethodMkcalendar = "MKCALENDAR"
	MethodMkcol      = "MKCOL"
	MethodMove       = "MOVE"
	MethodPropFind   = "PROPFIND"
	MethodPropPatch  = "PROPPATCH"
	MethodReport     = "REPORT"
	MethodUnlock     = "UNLOCK"
)
