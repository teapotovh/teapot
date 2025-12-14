// this file contains a lot of generated code for LDAP structures, inherited
// from the go-ldap library.
//
//nolint:staticcheck,unused
package message

type (
	OCTETSTRING string
	INTEGER     int32 // In this RFC the max INTEGER value is 2^31 - 1, so int32 is enough
	BOOLEAN     bool
	ENUMERATED  int32
)

// This appendix is normative.
//
//	Lightweight-Directory-Access-Protocol-V3 {1 3 6 1 1 18}
//	-- Copyright (C) The Internet Society (2006).  This version of
//	-- this ASN.1 module is part of RFC 4511; see the RFC itself
//	-- for full legal notices.
//	DEFINITIONS
//	IMPLICIT TAGS
//	EXTENSIBILITY IMPLIED ::=
//
//	BEGIN
//
//	LDAPMessage ::= SEQUENCE {
//	     messageID       MessageID,
//	     protocolOp      CHOICE {
//	          bindRequest           BindRequest,
//	          bindResponse          BindResponse,
//	          unbindRequest         UnbindRequest,
//	          searchRequest         SearchRequest,
//	          searchResEntry        SearchResultEntry,
//	          searchResDone         SearchResultDone,
//	          searchResRef          SearchResultReference,
//	          modifyRequest         ModifyRequest,
//	          modifyResponse        ModifyResponse,
//	          addRequest            AddRequest,
//	          addResponse           AddResponse,
//	          delRequest            DelRequest,
//	          delResponse           DelResponse,
//	          modDNRequest          ModifyDNRequest,
//	          modDNResponse         ModifyDNResponse,
//	          compareRequest        CompareRequest,
//	          compareResponse       CompareResponse,
//	          abandonRequest        AbandonRequest,
//	          extendedReq           ExtendedRequest,
//	          extendedResp          ExtendedResponse,
//	          ...,
//	          intermediateResponse  IntermediateResponse },
//	     controls       [0] Controls OPTIONAL }
type LDAPMessage struct {
	protocolOp ProtocolOp
	controls   *Controls
	messageID  MessageID
}

const TagLDAPMessageControls = 0

type ProtocolOp interface {
	size() int
	write(bytes *Bytes) int
}

// MessageID ::= INTEGER (0 ..  maxInt).
type MessageID INTEGER

// maxInt INTEGER ::= 2147483647 -- (2^^31 - 1) --.
const maxInt = INTEGER(2147483647)

// LDAPString ::= OCTET STRING -- UTF-8 encoded,
//
//	-- [ISO10646] characters
type LDAPString OCTETSTRING

// Sermersheim                 Standards Track                    [Page 54]
//
// # RFC 4511                         LDAPv3                        June 2006
//
//	LDAPOID ::= OCTET STRING -- Constrained to <numericoid>
//	                         -- [RFC4512]
type LDAPOID OCTETSTRING

// LDAPDN ::= LDAPString -- Constrained to <distinguishedName>
//
//	-- [RFC4514]
type LDAPDN LDAPString

// RelativeLDAPDN ::= LDAPString -- Constrained to <name-component>
//
//	-- [RFC4514]
type RelativeLDAPDN LDAPString

// AttributeDescription ::= LDAPString
//
//	-- Constrained to <attributedescription>
//	-- [RFC4512]
type AttributeDescription LDAPString

// AttributeValue ::= OCTET STRING.
type AttributeValue OCTETSTRING

//	AttributeValueAssertion ::= SEQUENCE {
//	     attributeDesc   AttributeDescription,
//	     assertionValue  AssertionValue }
type AttributeValueAssertion struct {
	attributeDesc  AttributeDescription
	assertionValue AssertionValue
}

// AssertionValue ::= OCTET STRING.
type AssertionValue OCTETSTRING

//	PartialAttribute ::= SEQUENCE {
//	     type       AttributeDescription,
//	     vals       SET OF value AttributeValue }
type PartialAttribute struct {
	type_ AttributeDescription
	vals  []AttributeValue
}

//	Attribute ::= PartialAttribute(WITH COMPONENTS {
//	     ...,
//	     vals (SIZE(1..MAX))})
type Attribute PartialAttribute

// MatchingRuleID ::= LDAPString.
type MatchingRuleID LDAPString

//	LDAPResult ::= SEQUENCE {
//	     resultCode         ENUMERATED {
//	          success                      (0),
//	          operationsError              (1),
//	          protocolError                (2),
//	          timeLimitExceeded            (3),
//	          sizeLimitExceeded            (4),
//	          compareFalse                 (5),
//	          compareTrue                  (6),
//	          authMethodNotSupported       (7),
//	          strongerAuthRequired         (8),
//	               -- 9 reserved --
//	          referral                     (10),
//	          adminLimitExceeded           (11),
//	          unavailableCriticalExtension (12),
//	          confidentialityRequired      (13),
//	          saslBindInProgress           (14),
//
// Sermersheim                 Standards Track                    [Page 55]
//
// # RFC 4511                         LDAPv3                        June 2006
//
//	     noSuchAttribute              (16),
//	     undefinedAttributeType       (17),
//	     inappropriateMatching        (18),
//	     constraintViolation          (19),
//	     attributeOrValueExists       (20),
//	     invalidAttributeSyntax       (21),
//	          -- 22-31 unused --
//	     noSuchObject                 (32),
//	     aliasProblem                 (33),
//	     invalidDNSyntax              (34),
//	          -- 35 reserved for undefined isLeaf --
//	     aliasDereferencingProblem    (36),
//	          -- 37-47 unused --
//	     inappropriateAuthentication  (48),
//	     invalidCredentials           (49),
//	     insufficientAccessRights     (50),
//	     busy                         (51),
//	     unavailable                  (52),
//	     unwillingToPerform           (53),
//	     loopDetect                   (54),
//	          -- 55-63 unused --
//	     namingViolation              (64),
//	     objectClassViolation         (65),
//	     notAllowedOnNonLeaf          (66),
//	     notAllowedOnRDN              (67),
//	     entryAlreadyExists           (68),
//	     objectClassModsProhibited    (69),
//	          -- 70 reserved for CLDAP --
//	     affectsMultipleDSAs          (71),
//	          -- 72-79 unused --
//	     other                        (80),
//	     ...  },
//	matchedDN          LDAPDN,
//	diagnosticMessage  LDAPString,
//	referral           [3] Referral OPTIONAL }
type LDAPResult struct {
	referral          *Referral
	matchedDN         LDAPDN
	diagnosticMessage LDAPString
	resultCode        ENUMERATED
}

const TagLDAPResultReferral = 3

const (
	ResultCodeSuccess                      = 0
	ResultCodeOperationsError              = 1
	ResultCodeProtocolError                = 2
	ResultCodeTimeLimitExceeded            = 3
	ResultCodeSizeLimitExceeded            = 4
	ResultCodeCompareFalse                 = 5
	ResultCodeCompareTrue                  = 6
	ResultCodeAuthMethodNotSupported       = 7
	ResultCodeStrongerAuthRequired         = 8
	ResultCodeReferral                     = 10
	ResultCodeAdminLimitExceeded           = 11
	ResultCodeUnavailableCriticalExtension = 12
	ResultCodeConfidentialityRequired      = 13
	ResultCodeSaslBindInProgress           = 14
	ResultCodeNoSuchAttribute              = 16
	ResultCodeUndefinedAttributeType       = 17
	ResultCodeInappropriateMatching        = 18
	ResultCodeConstraintViolation          = 19
	ResultCodeAttributeOrValueExists       = 20
	ResultCodeInvalidAttributeSyntax       = 21
	ResultCodeNoSuchObject                 = 32
	ResultCodeAliasProblem                 = 33
	ResultCodeInvalidDNSyntax              = 34
	ResultCodeAliasDereferencingProblem    = 36
	ResultCodeInappropriateAuthentication  = 48
	ResultCodeInvalidCredentials           = 49
	ResultCodeInsufficientAccessRights     = 50
	ResultCodeBusy                         = 51
	ResultCodeUnavailable                  = 52
	ResultCodeUnwillingToPerform           = 53
	ResultCodeLoopDetect                   = 54
	ResultCodeNamingViolation              = 64
	ResultCodeObjectClassViolation         = 65
	ResultCodeNotAllowedOnNonLeaf          = 66
	ResultCodeNotAllowedOnRDN              = 67
	ResultCodeEntryAlreadyExists           = 68
	ResultCodeObjectClassModsProhibited    = 69
	ResultCodeAffectsMultipleDSAs          = 71
	ResultCodeOther                        = 80
)

var EnumeratedLDAPResultCode = map[ENUMERATED]string{
	ResultCodeSuccess:                "success",
	ResultCodeOperationsError:        "operationsError",
	ResultCodeProtocolError:          "protocolError",
	ResultCodeTimeLimitExceeded:      "timeLimitExceeded",
	ResultCodeSizeLimitExceeded:      "sizeLimitExceeded",
	ResultCodeCompareFalse:           "compareFalse",
	ResultCodeCompareTrue:            "compareTrue",
	ResultCodeAuthMethodNotSupported: "authMethodNotSupported",
	ResultCodeStrongerAuthRequired:   "strongerAuthRequired",
	//                       -- 9 reserved --
	ResultCodeReferral:                     "referral",
	ResultCodeAdminLimitExceeded:           "adminLimitExceeded",
	ResultCodeUnavailableCriticalExtension: "unavailableCriticalExtension",
	ResultCodeConfidentialityRequired:      "confidentialityRequired",
	ResultCodeSaslBindInProgress:           "saslBindInProgress",
	ResultCodeNoSuchAttribute:              "noSuchAttribute",
	ResultCodeUndefinedAttributeType:       "undefinedAttributeType",
	ResultCodeInappropriateMatching:        "inappropriateMatching",
	ResultCodeConstraintViolation:          "constraintViolation",
	ResultCodeAttributeOrValueExists:       "attributeOrValueExists",
	ResultCodeInvalidAttributeSyntax:       "invalidAttributeSyntax",
	//                       -- 22-31 unused --
	ResultCodeNoSuchObject:    "noSuchObject",
	ResultCodeAliasProblem:    "aliasProblem",
	ResultCodeInvalidDNSyntax: "invalidDNSyntax",
	//                       -- 35 reserved for undefined isLeaf --
	ResultCodeAliasDereferencingProblem: "aliasDereferencingProblem",
	//                       -- 37-47 unused --
	ResultCodeInappropriateAuthentication: "inappropriateAuthentication",
	ResultCodeInvalidCredentials:          "invalidCredentials",
	ResultCodeInsufficientAccessRights:    "insufficientAccessRights",
	ResultCodeBusy:                        "busy",
	ResultCodeUnavailable:                 "unavailable",
	ResultCodeUnwillingToPerform:          "unwillingToPerform",
	ResultCodeLoopDetect:                  "loopDetect",
	//                       -- 55-63 unused --
	ResultCodeNamingViolation:           "namingViolation",
	ResultCodeObjectClassViolation:      "objectClassViolation",
	ResultCodeNotAllowedOnNonLeaf:       "notAllowedOnNonLeaf",
	ResultCodeNotAllowedOnRDN:           "notAllowedOnRDN",
	ResultCodeEntryAlreadyExists:        "entryAlreadyExists",
	ResultCodeObjectClassModsProhibited: "objectClassModsProhibited",
	//                       -- 70 reserved for CLDAP --
	ResultCodeAffectsMultipleDSAs: "affectsMultipleDSAs",
	//                       -- 72-79 unused --
	ResultCodeOther: "other",
}

// Referral ::= SEQUENCE SIZE (1..MAX) OF uri URI.
type Referral []URI

// URI ::= LDAPString     -- limited to characters permitted in
//
//	-- URIs
type URI LDAPString

// Controls ::= SEQUENCE OF control Control.
type Controls []Control

//	Control ::= SEQUENCE {
//	     controlType             LDAPOID,
//	     criticality             BOOLEAN DEFAULT FALSE,
//	     controlValue            OCTET STRING OPTIONAL }
type Control struct {
	controlValue *OCTETSTRING
	controlType  LDAPOID
	criticality  BOOLEAN
}

// Sermersheim                 Standards Track                    [Page 56]
//
// # RFC 4511                         LDAPv3                        June 2006
//
//	BindRequest ::= [APPLICATION 0] SEQUENCE {
//	     version                 INTEGER (1 ..  127),
//	     name                    LDAPDN,
//	     authentication          AuthenticationChoice }
const (
	TagBindRequest        = 0
	BindRequestVersionMin = 1
	BindRequestVersionMax = 127
)

type BindRequest struct {
	authentication AuthenticationChoice
	name           LDAPDN
	version        INTEGER
}

//	AuthenticationChoice ::= CHOICE {
//	     simple                  [0] OCTET STRING,
//	                             -- 1 and 2 reserved
//	     sasl                    [3] SaslCredentials,
//	     ...  }
const (
	TagAuthenticationChoiceSimple          = 0
	TagAuthenticationChoiceSaslCredentials = 3
)

type AuthenticationChoice interface {
	sizeTagged(tag int) int
}

//	SaslCredentials ::= SEQUENCE {
//	     mechanism               LDAPString,
//	     credentials             OCTET STRING OPTIONAL }
type SaslCredentials struct {
	credentials *OCTETSTRING
	mechanism   LDAPString
}

//	BindResponse ::= [APPLICATION 1] SEQUENCE {
//	     COMPONENTS OF LDAPResult,
//	     serverSaslCreds    [7] OCTET STRING OPTIONAL }
const (
	TagBindResponse                = 1
	TagBindResponseServerSaslCreds = 7
)

type BindResponse struct {
	LDAPResult

	serverSaslCreds *OCTETSTRING
}

// UnbindRequest ::= [APPLICATION 2] NULL.
const TagUnbindRequest = 2

type UnbindRequest struct{}

//	PasswdModifyRequestValue ::= SEQUENCE {
//	  userIdentity    [0]  OCTET STRING OPTIONAL
//	  oldPasswd       [1]  OCTET STRING OPTIONAL
//	  newPasswd       [2]  OCTET STRING OPTIONAL }
type PasswordModifyRequest struct {
	userIdentity *OCTETSTRING
	oldPassword  *OCTETSTRING
	newPassword  *OCTETSTRING
}

const (
	TagPasswordModifyRequestUserIdentity = 0
	TagPasswordModifyRequestOldPassword  = 1
	TagPasswordModifyRequestNewPassword  = 2
)

//	PasswdModifyResponseValue ::= SEQUENCE {
//	  genPasswd       [0]     OCTET STRING OPTIONAL }
type PasswordModifyResponse struct {
	genPassword *OCTETSTRING
}

//	SearchRequest ::= [APPLICATION 3] SEQUENCE {
//	     baseObject      LDAPDN,
//	     scope           ENUMERATED {
//	          baseObject              (0),
//	          singleLevel             (1),
//	          wholeSubtree            (2),
//	          ...  },
//	     derefAliases    ENUMERATED {
//	          neverDerefAliases       (0),
//	          derefInSearching        (1),
//	          derefFindingBaseObj     (2),
//	          derefAlways             (3) },
//	     sizeLimit       INTEGER (0 ..  maxInt),
//	     timeLimit       INTEGER (0 ..  maxInt),
//	     typesOnly       BOOLEAN,
//	     filter          Filter,
//	     attributes      AttributeSelection }
const TagSearchRequest = 3

type SearchRequest struct {
	filter       Filter
	baseObject   LDAPDN
	attributes   AttributeSelection
	scope        ENUMERATED
	derefAliases ENUMERATED
	sizeLimit    INTEGER
	timeLimit    INTEGER
	typesOnly    BOOLEAN
}

const (
	SearchRequestScopeBaseObject = 0
	SearchRequestSingleLevel     = 1
	SearchRequestHomeSubtree     = 2
)

var EnumeratedSearchRequestScope = map[ENUMERATED]string{
	SearchRequestScopeBaseObject: "baseObject",
	SearchRequestSingleLevel:     "singleLevel",
	SearchRequestHomeSubtree:     "homeSubtree",
}

const (
	SearchRequetDerefAliasesNeverDerefAliases   = 0
	SearchRequetDerefAliasesDerefInSearching    = 1
	SearchRequetDerefAliasesDerefFindingBaseObj = 2
	SearchRequetDerefAliasesDerefAlways         = 3
)

var EnumeratedSearchRequestDerefAliases = map[ENUMERATED]string{
	SearchRequetDerefAliasesNeverDerefAliases:   "neverDerefAliases",
	SearchRequetDerefAliasesDerefInSearching:    "derefInSearching",
	SearchRequetDerefAliasesDerefFindingBaseObj: "derefFindingBaseObj",
	SearchRequetDerefAliasesDerefAlways:         "derefAlways",
}

// AttributeSelection ::= SEQUENCE OF selector LDAPString
//
//	-- The LDAPString is constrained to
//	-- <attributeSelector> in Section 4.5.1.8
type AttributeSelection []LDAPString

//	Filter ::= CHOICE {
//	     and             [0] SET SIZE (1..MAX) OF filter Filter,
//	     or              [1] SET SIZE (1..MAX) OF filter Filter,
//	     not             [2] Filter,
//	     equalityMatch   [3] AttributeValueAssertion,
//
// Sermersheim                 Standards Track                    [Page 57]
//
// # RFC 4511                         LDAPv3                        June 2006
//
//	substrings      [4] SubstringFilter,
//	greaterOrEqual  [5] AttributeValueAssertion,
//	lessOrEqual     [6] AttributeValueAssertion,
//	present         [7] AttributeDescription,
//	approxMatch     [8] AttributeValueAssertion,
//	extensibleMatch [9] MatchingRuleAssertion,
//	...  }
const (
	TagFilterAnd             = 0
	TagFilterOr              = 1
	TagFilterNot             = 2
	TagFilterEqualityMatch   = 3
	TagFilterSubstrings      = 4
	TagFilterGreaterOrEqual  = 5
	TagFilterLessOrEqual     = 6
	TagFilterPresent         = 7
	TagFilterApproxMatch     = 8
	TagFilterExtensibleMatch = 9
)

type Filter interface {
	size() int
	write(bytes *Bytes) int
	getFilterTag() int
}
type (
	FilterAnd []Filter
	FilterOr  []Filter
	FilterNot struct {
		Filter
	}
)

type (
	FilterEqualityMatch   AttributeValueAssertion
	FilterSubstrings      SubstringFilter
	FilterGreaterOrEqual  AttributeValueAssertion
	FilterLessOrEqual     AttributeValueAssertion
	FilterPresent         AttributeDescription
	FilterApproxMatch     AttributeValueAssertion
	FilterExtensibleMatch MatchingRuleAssertion
)

//	SubstringFilter ::= SEQUENCE {
//	     type           AttributeDescription,
//	     substrings     SEQUENCE SIZE (1..MAX) OF substring CHOICE {
//	          initial [0] AssertionValue,  -- can occur at most once
//	          any     [1] AssertionValue,
//	          final   [2] AssertionValue } -- can occur at most once
//	     }
type SubstringFilter struct {
	type_      AttributeDescription
	substrings []Substring
}

type Substring any

const (
	TagSubstringInitial = 0
	TagSubstringAny     = 1
	TagSubstringFinal   = 2
)

type (
	SubstringInitial AssertionValue
	SubstringAny     AssertionValue
	SubstringFinal   AssertionValue
)

//	MatchingRuleAssertion ::= SEQUENCE {
//	     matchingRule    [1] MatchingRuleId OPTIONAL,
//	     type            [2] AttributeDescription OPTIONAL,
//	     matchValue      [3] AssertionValue,
//	     dnAttributes    [4] BOOLEAN DEFAULT FALSE }
type MatchingRuleAssertion struct {
	matchingRule *MatchingRuleID
	type_        *AttributeDescription
	matchValue   AssertionValue
	dnAttributes BOOLEAN
}

const (
	TagMatchingRuleAssertionMatchingRule = 1
	TagMatchingRuleAssertionType         = 2
	TagMatchingRuleAssertionMatchValue   = 3
	TagMatchingRuleAssertionDnAttributes = 4
)

//	SearchResultEntry ::= [APPLICATION 4] SEQUENCE {
//	     objectName      LDAPDN,
//	     attributes      PartialAttributeList }
const TagSearchResultEntry = 4

type SearchResultEntry struct {
	objectName LDAPDN
	attributes PartialAttributeList
}

// PartialAttributeList ::= SEQUENCE OF
//
//	partialAttribute PartialAttribute
type PartialAttributeList []PartialAttribute

// SearchResultReference ::= [APPLICATION 19] SEQUENCE
//
//	SIZE (1..MAX) OF uri URI
const TagSearchResultReference = 19

type SearchResultReference []URI

// SearchResultDone ::= [APPLICATION 5] LDAPResult.
const TagSearchResultDone = 5

type SearchResultDone LDAPResult

//	ModifyRequest ::= [APPLICATION 6] SEQUENCE {
//	     object          LDAPDN,
//	     changes         SEQUENCE OF change SEQUENCE {
//	          operation       ENUMERATED {
//	               add     (0),
//	               delete  (1),
//	               replace (2),
//	               ...  },
//	          modification    PartialAttribute } }
const TagModifyRequest = 6

type ModifyRequest struct {
	object  LDAPDN
	changes []ModifyRequestChange
}
type ModifyRequestChange struct {
	modification PartialAttribute
	operation    ENUMERATED
}

const (
	ModifyRequestChangeOperationAdd     = 0
	ModifyRequestChangeOperationDelete  = 1
	ModifyRequestChangeOperationReplace = 2
)

var EnumeratedModifyRequestChangeOperation = map[ENUMERATED]string{
	ModifyRequestChangeOperationAdd:     "add",
	ModifyRequestChangeOperationDelete:  "delete",
	ModifyRequestChangeOperationReplace: "replace",
}

// ModifyResponse ::= [APPLICATION 7] LDAPResult.
const TagModifyResponse = 7

type ModifyResponse LDAPResult

// Sermersheim                 Standards Track                    [Page 58]
//
// # RFC 4511                         LDAPv3                        June 2006
//
//	AddRequest ::= [APPLICATION 8] SEQUENCE {
//	     entry           LDAPDN,
//	     attributes      AttributeList }
const TagAddRequest = 8

type AddRequest struct {
	entry      LDAPDN
	attributes AttributeList
}

// AttributeList ::= SEQUENCE OF attribute Attribute.
type AttributeList []Attribute

// AddResponse ::= [APPLICATION 9] LDAPResult.
const TagAddResponse = 9

type AddResponse LDAPResult

// DelRequest ::= [APPLICATION 10] LDAPDN.
const TagDelRequest = 10

type DelRequest LDAPDN

// DelResponse ::= [APPLICATION 11] LDAPResult.
const TagDelResponse = 11

type DelResponse LDAPResult

//	ModifyDNRequest ::= [APPLICATION 12] SEQUENCE {
//	     entry           LDAPDN,
//	     newrdn          RelativeLDAPDN,
//	     deleteoldrdn    BOOLEAN,
//	     newSuperior     [0] LDAPDN OPTIONAL }
const TagModifyDNRequest = 12

type ModifyDNRequest struct {
	newSuperior  *LDAPDN
	entry        LDAPDN
	newrdn       RelativeLDAPDN
	deleteoldrdn BOOLEAN
}

const TagModifyDNRequestNewSuperior = 0

// ModifyDNResponse ::= [APPLICATION 13] LDAPResult.
const TagModifyDNResponse = 13

type ModifyDNResponse LDAPResult

//	CompareRequest ::= [APPLICATION 14] SEQUENCE {
//	     entry           LDAPDN,
//	     ava             AttributeValueAssertion }
const TagCompareRequest = 14

type CompareRequest struct {
	entry LDAPDN
	ava   AttributeValueAssertion
}

// CompareResponse ::= [APPLICATION 15] LDAPResult.
const TagCompareResponse = 15

type CompareResponse LDAPResult

// AbandonRequest ::= [APPLICATION 16] MessageID.
const TagAbandonRequest = 16

type AbandonRequest MessageID

//	ExtendedRequest ::= [APPLICATION 23] SEQUENCE {
//	     requestName      [0] LDAPOID,
//	     requestValue     [1] OCTET STRING OPTIONAL }
const TagExtendedRequest = 23

type ExtendedRequest struct {
	requestValue *OCTETSTRING
	requestName  LDAPOID
}

const (
	TagExtendedRequestName  = 0
	TagExtendedRequestValue = 1
)

//	ExtendedResponse ::= [APPLICATION 24] SEQUENCE {
//	     COMPONENTS OF LDAPResult,
//	     responseName     [10] LDAPOID OPTIONAL,
//	     responseValue    [11] OCTET STRING OPTIONAL }
const TagExtendedResponse = 24

type ExtendedResponse struct {
	LDAPResult

	responseName  *LDAPOID
	responseValue *OCTETSTRING
}

const (
	TagExtendedResponseName  = 10
	TagExtendedResponseValue = 11
)

//	IntermediateResponse ::= [APPLICATION 25] SEQUENCE {
//	     responseName     [0] LDAPOID OPTIONAL,
//	     responseValue    [1] OCTET STRING OPTIONAL }
const TagIntermediateResponse = 25

type IntermediateResponse struct {
	responseName  *LDAPOID
	responseValue *OCTETSTRING
}

const (
	TagIntermediateResponseName  = 0
	TagIntermediateResponseValue = 1
)

//
//        END
//
