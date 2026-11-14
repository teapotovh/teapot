package webdav

import "github.com/teapotovh/teapot/lib/webdav/internal"

// WebDAV and CalDAV method names that are not in net/http.
const (
	MethodACL        = internal.MethodACL
	MethodCopy       = internal.MethodCopy
	MethodLock       = internal.MethodLock
	MethodMkcalendar = internal.MethodMkcalendar
	MethodMkcol      = internal.MethodMkcol
	MethodMove       = internal.MethodMove
	MethodPropFind   = internal.MethodPropFind
	MethodPropPatch  = internal.MethodPropPatch
	MethodReport     = internal.MethodReport
	MethodUnlock     = internal.MethodUnlock
)
