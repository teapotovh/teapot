package ldapsrv

import (
	"errors"
	"fmt"

	ldap "github.com/teapotovh/teapot/lib/ldapsrv/goldap"
)

var (
	ErrOperationsError              = withCode(ldap.ResultCodeOperationsError, "operations error")
	ErrProtocolError                = withCode(ldap.ResultCodeProtocolError, "protocol error")
	ErrTimeLimitExceeded            = withCode(ldap.ResultCodeTimeLimitExceeded, "time limit exceeded")
	ErrSizeLimitExceeded            = withCode(ldap.ResultCodeSizeLimitExceeded, "size limit exceeded")
	ErrCompareFalse                 = withCode(ldap.ResultCodeCompareFalse, "compare false")
	ErrCompareTrue                  = withCode(ldap.ResultCodeCompareTrue, "compare true")
	ErrAuthMethodNotSupported       = withCode(ldap.ResultCodeAuthMethodNotSupported, "auth method not supported")
	ErrStrongerAuthRequired         = withCode(ldap.ResultCodeStrongerAuthRequired, "stronger auth required")
	ErrReferral                     = withCode(ldap.ResultCodeReferral, "referral")
	ErrAdminLimitExceeded           = withCode(ldap.ResultCodeAdminLimitExceeded, "admin limit exceeded")
	ErrUnavailableCriticalExtension = withCode(ldap.ResultCodeUnavailableCriticalExtension, "unavailable critical extension")
	ErrConfidentialityRequired      = withCode(ldap.ResultCodeConfidentialityRequired, "confidentiality required")
	ErrSaslBindInProgress           = withCode(ldap.ResultCodeSaslBindInProgress, "sasl bind in progress")
	ErrNoSuchAttribute              = withCode(ldap.ResultCodeNoSuchAttribute, "no such attribute")
	ErrUndefinedAttributeType       = withCode(ldap.ResultCodeUndefinedAttributeType, "undefined attribute type")
	ErrInappropriateMatching        = withCode(ldap.ResultCodeInappropriateMatching, "inappropriate matching")
	ErrConstraintViolation          = withCode(ldap.ResultCodeConstraintViolation, "constraint violation")
	ErrAttributeOrValueExists       = withCode(ldap.ResultCodeAttributeOrValueExists, "attribute or value exists")
	ErrInvalidAttributeSyntax       = withCode(ldap.ResultCodeInvalidAttributeSyntax, "invalid attribute syntax")
	ErrNoSuchObject                 = withCode(ldap.ResultCodeNoSuchObject, "no such object")
	ErrAliasProblem                 = withCode(ldap.ResultCodeAliasProblem, "alias problem")
	ErrInvalidDNSyntax              = withCode(ldap.ResultCodeInvalidDNSyntax, "invalid DN syntax")
	ErrAliasDereferencingProblem    = withCode(ldap.ResultCodeAliasDereferencingProblem, "alias dereference problem")
	ErrInappropriateAuthentication  = withCode(ldap.ResultCodeInappropriateAuthentication, "inappropriate authentication")
	ErrInvalidCredentials           = withCode(ldap.ResultCodeInvalidCredentials, "invalid credentials")
	ErrInsufficientAccessRights     = withCode(ldap.ResultCodeInsufficientAccessRights, "insufficient access rights")
	ErrBusy                         = withCode(ldap.ResultCodeBusy, "busy")
	ErrUnavailable                  = withCode(ldap.ResultCodeUnavailable, "unavailable")
	ErrUnwillingToPerform           = withCode(ldap.ResultCodeUnwillingToPerform, "unwilling to perform")
	ErrLoopDetect                   = withCode(ldap.ResultCodeLoopDetect, "loop detected")
	ErrNamingViolation              = withCode(ldap.ResultCodeNamingViolation, "naming violation")
	ErrObjectClassViolation         = withCode(ldap.ResultCodeObjectClassViolation, "object class violation")
	ErrNotAllowedOnNonLeaf          = withCode(ldap.ResultCodeNotAllowedOnNonLeaf, "not allowed on non leaf")
	ErrNotAllowedOnRDN              = withCode(ldap.ResultCodeNotAllowedOnRDN, "not allowed on RDN")
	ErrEntryAlreadyExists           = withCode(ldap.ResultCodeEntryAlreadyExists, "entry already exists")
	ErrObjectClassModsProhibited    = withCode(ldap.ResultCodeObjectClassModsProhibited, "object class mods prohibited")
	ErrAffectsMultipleDSAs          = withCode(ldap.ResultCodeAffectsMultipleDSAs, "affects multiple DSAs")
	ErrOther                        = withCode(ldap.ResultCodeOther, "other")

	ErrUnimplemented = fmt.Errorf("(%w) operation not unimplemented", ErrUnwillingToPerform)
	ErrUnsupported   = fmt.Errorf("(%w) operation not supported", ErrUnwillingToPerform)
)

type codeError struct {
	code ldap.ENUMERATED
	error
}

func withCode(code ldap.ENUMERATED, message string) error {
	return codeError{code: code, error: errors.New(message)}
}

func (ce codeError) LDAPCode() ldap.ENUMERATED {
	return ce.code
}
