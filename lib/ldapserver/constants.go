package ldapserver

import ldap "github.com/teapotovh/teapot/lib/ldapserver/goldap"

// LDAP Application Codes.
const (
	ApplicationBindRequest           = 0
	ApplicationBindResponse          = 1
	ApplicationUnbindRequest         = 2
	ApplicationSearchRequest         = 3
	ApplicationSearchResultEntry     = 4
	ApplicationSearchResultDone      = 5
	ApplicationModifyRequest         = 6
	ApplicationModifyResponse        = 7
	ApplicationAddRequest            = 8
	ApplicationAddResponse           = 9
	ApplicationDelRequest            = 10
	ApplicationDelResponse           = 11
	ApplicationModifyDNRequest       = 12
	ApplicationModifyDNResponse      = 13
	ApplicationCompareRequest        = 14
	ApplicationCompareResponse       = 15
	ApplicationAbandonRequest        = 16
	ApplicationSearchResultReference = 19
	ApplicationExtendedRequest       = 23
	ApplicationExtendedResponse      = 24
)

// Modify Request Operation code.
const (
	ModifyRequestChangeOperationAdd     = 0
	ModifyRequestChangeOperationDelete  = 1
	ModifyRequestChangeOperationReplace = 2
)

const (
	SearchRequestScopeBaseObject = 0
	SearchRequestSingleLevel     = 1
	SearchRequestHomeSubtree     = 2
)

// Extended operation responseName and requestName.
const (
	NoticeOfDisconnection   ldap.LDAPOID = "1.3.6.1.4.1.1466.2003"
	NoticeOfCancel          ldap.LDAPOID = "1.3.6.1.1.8"
	NoticeOfStartTLS        ldap.LDAPOID = "1.3.6.1.4.1.1466.20037"
	NoticeOfWhoAmI          ldap.LDAPOID = "1.3.6.1.4.1.4203.1.11.3"
	NoticeOfGetConnectionID ldap.LDAPOID = "1.3.6.1.4.1.26027.1.6.2"
	NoticeOfPasswordModify  ldap.LDAPOID = "1.3.6.1.4.1.4203.1.11.1"
)
