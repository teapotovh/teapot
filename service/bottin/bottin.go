package bottin

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"

	"github.com/google/uuid"

	"github.com/teapotovh/teapot/lib/ldapserver"
	ldap "github.com/teapotovh/teapot/lib/ldapserver/goldap"
	"github.com/teapotovh/teapot/service/bottin/store"
)

const (
	// System managed attributes (cannot be changed by user, see checkRestrictedAttr).

	AttrMemberOf        store.AttributeKey = "memberof"
	AttrEntryUUID       store.AttributeKey = "entryuuid"
	AttrCreatorsName    store.AttributeKey = "creatorsname"
	AttrCreateTimestamp store.AttributeKey = "createtimestamp"
	AttrModifiersName   store.AttributeKey = "modifiersname"
	AttrModifyTimestamp store.AttributeKey = "modifytimestamp"

	// Attributes that we are interested in at various points.

	AttrObjectClass  store.AttributeKey = "objectclass"
	AttrMember       store.AttributeKey = "member"
	AttrUserPassword store.AttributeKey = "userpassword"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrInsufficientRights = errors.New("insufficient access rights")
	ErrSetRootPasswd      = errors.New("root entry password cannot be set")
	ErrNotAuthenticated   = errors.New("not authenticated")
	ErrMissingNewPassword = errors.New("new password is missing")
	ErrDNNotPrefix        = errors.New("DN is not a prefix of base DN")
)

type BottinConfig struct {
	Store         store.StoreConfig
	BaseDN        string
	Passwd        string
	TLSCertFile   string
	TLSKeyFile    string
	TLSServerName string
	ACL           []string
}

type Bottin struct {
	store      store.Store
	logger     *slog.Logger
	rootPasswd string
	baseDN     store.DN
	acl        ACL
}

func NewBottin(config BottinConfig, logger *slog.Logger) (*Bottin, error) {
	acl, err := parseACL(config.ACL, logger)
	if err != nil {
		return nil, fmt.Errorf("error while parsing ACL: %w", err)
	}

	baseDN, err := store.ParseDN(config.BaseDN)
	if err != nil {
		return nil, fmt.Errorf("error while parsing baseDN: %w", err)
	}

	hash, err := ssha512Encode(config.Passwd)
	if err != nil {
		return nil, fmt.Errorf("error while hashing root passwd: %w", err)
	}

	store, err := store.NewStore(config.Store)
	if err != nil {
		return nil, fmt.Errorf("error while initializing bottin store: %w", err)
	}

	return &Bottin{
		logger:     slog.New(NewContextHandler(logger.Handler())),
		baseDN:     baseDN,
		rootPasswd: hash,
		acl:        acl,
		store:      store,
	}, nil
}

const AnonymousUser = "ANONYMOUS"

func EmptyUser() User {
	return User{
		user:   AnonymousUser,
		groups: []string{},
	}
}

func (server *Bottin) Init(ctx context.Context) error {
	// Check that root object exists.
	// If it does, we're done. Otherwise, we have some initialization to do.
	exists, err := server.existsEntry(ctx, server.baseDN)
	if err != nil {
		return fmt.Errorf("error while checking for root object existence: %w", err)
	}

	if exists {
		return nil
	}

	uuid, err := uuid.NewRandom()
	if err != nil {
		return fmt.Errorf("error while generating random uuid: %w", err)
	}

	// We have to initialize the server. Create a root object.
	baseAttributes := store.Attributes{
		AttrObjectClass:         store.AttributeValue{"top", "dcObject", "organization"},
		"structuralobjectclass": store.AttributeValue{"organization"},
		AttrCreatorsName:        store.AttributeValue{server.baseDN.String()},
		AttrCreateTimestamp:     store.AttributeValue{genTimestamp()},
		AttrEntryUUID:           store.AttributeValue{uuid.String()},
	}

	tx, err := server.store.Begin(ctx)
	if err != nil {
		return fmt.Errorf("error while beginning store transaction: %w", err)
	}

	entry := store.NewEntry(server.baseDN, baseAttributes)
	if err = tx.Store(entry); err != nil {
		return fmt.Errorf("error while storing base entry: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("could not commit store transaction: %w", err)
	}

	server.logger.InfoContext(ctx, "initialized base entry", "dn", server.baseDN)

	return nil
}

func (server *Bottin) HandlePasswordModify(
	ctx context.Context,
	w ldapserver.ResponseWriter,
	m *ldapserver.Message,
) context.Context {
	r := m.GetExtendedRequest()
	resultCode, err := server.handlePasswordModifyInternal(ctx, &r)

	res := ldapserver.NewExtendedResponse(resultCode)
	res.SetResponseName(ldapserver.NoticeOfPasswordModify)

	if err != nil {
		res.SetDiagnosticMessage(err.Error())
	}

	if resultCode == ldap.ResultCodeSuccess {
		server.logger.InfoContext(ctx, "passwd successful")
	} else {
		server.logger.InfoContext(ctx, "passwd failed", "err", err)
	}

	w.Write(res)

	return ctx
}

func (server *Bottin) HandleBind(
	ctx context.Context,
	w ldapserver.ResponseWriter,
	m *ldapserver.Message,
) context.Context {
	r := m.GetBindRequest()
	ctx, resultCode, err := server.handleBindInternal(ctx, &r)

	res := ldapserver.NewBindResponse(resultCode)
	if err != nil {
		res.SetDiagnosticMessage(err.Error())
	}

	if resultCode == ldap.ResultCodeSuccess {
		server.logger.InfoContext(ctx, "bind successful", "user", r.Name())
	} else {
		server.logger.InfoContext(ctx, "bind failed", "user", r.Name(), "err", err)
	}

	w.Write(res)

	return ctx
}

func (server *Bottin) parseDN(rawDN string, allowPrefix bool) (store.DN, error) {
	dn, err := store.ParseDN(rawDN)
	if err != nil {
		return nil, err
	}

	baseDN := server.baseDN
	basePrefix := baseDN.Prefix()

	if basePrefix.IsPrefixOf(dn.Prefix()) {
		// Fast path: dn is a prefix of the baseDN, all is well
		return dn, nil
	}

	// If dn is allowed to be a prefix of baseDN, check for that
	if allowPrefix && dn.Prefix().IsPrefixOf(basePrefix) {
		// It is safe to return the baseDN, there's nothing outside of it anyway
		return baseDN, nil
	}

	return nil, fmt.Errorf("could not parse DN %q (base %q): %w", dn.String(), baseDN.String(), ErrDNNotPrefix)
}

func (server *Bottin) handlePasswordModifyInternal(ctx context.Context, r *ldap.ExtendedRequest) (int32, error) {
	passwordModifyRequest, err := r.PasswordModifyRequest()
	if err != nil {
		return ldap.ResultCodeInvalidAttributeSyntax, fmt.Errorf("error while parsing PasswordModify: %w", err)
	}

	if passwordModifyRequest.NewPassword() == nil {
		return ldap.ResultCodeAuthMethodNotSupported, ErrMissingNewPassword
	}

	passwd := passwordModifyRequest.NewPassword()

	user := ldapserver.GetUser(ctx, EmptyUser)
	if user.user == AnonymousUser {
		return ldap.ResultCodeInsufficientAccessRights, ErrNotAuthenticated
	}
	// By default we assume a user is trying to change his own password.
	// If a different subject is specified in the request, then we pivot to changing
	// the password for that subject instead.
	rawDN := user.user
	if passwordModifyRequest.UserIdentity() != nil {
		rawDN = string(*passwordModifyRequest.UserIdentity())
	}

	dn, err := server.parseDN(rawDN, false)
	if err != nil {
		return ldap.ResultCodeInvalidDNSyntax, err
	}

	// Check permissions
	if !server.acl.Check(user, "modify", dn, []store.AttributeKey{AttrUserPassword}) {
		return ldap.ResultCodeInsufficientAccessRights, fmt.Errorf(
			"could not modify password for %q: %w",
			dn,
			ErrInsufficientRights,
		)
	}

	if dn.Equal(server.baseDN) {
		return ldap.ResultCodeInvalidDNSyntax, ErrSetRootPasswd
	}

	entry, err := server.getEntry(ctx, dn)
	if err != nil {
		return ldap.ResultCodeOperationsError, err
	}

	hash, err := ssha512Encode(string(*passwd))
	if err != nil {
		return ldap.ResultCodeOperationsError, fmt.Errorf("error while hashing passwd: %w", err)
	}

	server.logger.InfoContext(ctx, "updating passwd", "dn", dn, "hash", hash)

	attrs := maps.Clone(entry.Attributes)
	attrs[AttrUserPassword] = store.AttributeValue{hash}

	tx, err := server.store.Begin(ctx)
	if err != nil {
		return ldap.ResultCodeOperationsError, fmt.Errorf("error while beginning transaction: %w", err)
	}

	if err = tx.Store(store.NewEntry(dn, attrs)); err != nil {
		return ldap.ResultCodeOperationsError, fmt.Errorf("error while updating password: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return ldap.ResultCodeOperationsError, fmt.Errorf("could not commit transaction: %w", err)
	}

	return ldap.ResultCodeSuccess, nil
}

func (server *Bottin) handleBindInternal(ctx context.Context, r *ldap.BindRequest) (context.Context, int32, error) {
	user := ldapserver.GetUser(ctx, EmptyUser)

	dn, err := server.parseDN(string(r.Name()), false)
	if err != nil {
		return nil, ldap.ResultCodeInvalidDNSyntax, err
	}

	server.logger.InfoContext(ctx, "bind attempt", "dn", dn, "user", user)

	// Check permissions
	if !server.acl.Check(user, "bind", dn, []store.AttributeKey{}) {
		return ctx, ldap.ResultCodeInsufficientAccessRights, fmt.Errorf(
			"could not authentiate as %q: %w",
			dn,
			ErrInsufficientRights,
		)
	}

	entry, err := server.getEntry(ctx, dn)
	if err != nil {
		return ctx, ldap.ResultCodeInvalidCredentials, err
	}

	passwd := string(r.AuthenticationSimple())

	var hashes store.AttributeValue
	if dn.Equal(server.baseDN) {
		hashes = store.AttributeValue{server.rootPasswd}
	} else {
		hashes = entry.Get(AttrUserPassword)
	}

	for _, hash := range hashes {
		server.logger.InfoContext(ctx, "matching against hash", "hash", hash)

		valid, err := matchPassword(hash, passwd)
		if err != nil {
			return ctx, ldap.ResultCodeInvalidCredentials, fmt.Errorf("can't authenticate: %w", err)
		}

		if valid {
			groups := entry.Get(AttrMemberOf)
			ctx = ldapserver.WithUser(ctx, User{
				user:   string(r.Name()),
				groups: groups,
			})

			return ctx, ldap.ResultCodeSuccess, nil
		}
	}

	return ctx, ldap.ResultCodeInvalidCredentials, ErrInvalidCredentials
}
