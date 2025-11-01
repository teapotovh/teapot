package bottin

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"os"

	"github.com/google/uuid"
	"github.com/teapotovh/teapot/lib/ldapserver"
	ldap "github.com/teapotovh/teapot/lib/ldapserver/goldap"
	"github.com/teapotovh/teapot/service/bottin/store"
)

const (
	// System managed attributes (cannot be changed by user, see checkRestrictedAttr)

	ATTR_MEMBEROF        store.AttributeKey = "memberof"
	ATTR_ENTRYUUID       store.AttributeKey = "entryuuid"
	ATTR_CREATORSNAME    store.AttributeKey = "creatorsname"
	ATTR_CREATETIMESTAMP store.AttributeKey = "createtimestamp"
	ATTR_MODIFIERSNAME   store.AttributeKey = "modifiersname"
	ATTR_MODIFYTIMESTAMP store.AttributeKey = "modifytimestamp"

	// Attributes that we are interested in at various points

	ATTR_OBJECTCLASS  store.AttributeKey = "objectclass"
	ATTR_MEMBER       store.AttributeKey = "member"
	ATTR_USERPASSWORD store.AttributeKey = "userpassword"
)

type BottinConfig struct {
	BaseDN string
	Passwd string
	ACL    []string

	TLSCertFile   string
	TLSKeyFile    string
	TLSServerName string

	Store store.StoreConfig
}

type Bottin struct {
	logger     *slog.Logger
	baseDN     store.DN
	rootPasswd string
	acl        ACL
	store      store.Store

	tlsConfig *tls.Config
}

func NewBottin(config BottinConfig, logger *slog.Logger) (*Bottin, error) {
	acl, err := parseACL(config.ACL)
	if err != nil {
		return nil, fmt.Errorf("error while parsing ACL: %w", err)
	}

	baseDN, err := store.ParseDN(config.BaseDN)
	if err != nil {
		return nil, fmt.Errorf("error while parsing baseDN: %w", err)
	}

	var tlsConfig *tls.Config = nil
	if config.TLSCertFile != "" && config.TLSKeyFile != "" && config.TLSServerName != "" {
		cert_txt, err := os.ReadFile(config.TLSCertFile)
		if err != nil {
			return nil, fmt.Errorf("error while reaing TLS cert at %s: %w", config.TLSCertFile, err)
		}
		key_txt, err := os.ReadFile(config.TLSKeyFile)
		if err != nil {
			return nil, fmt.Errorf("error while reaing TLS key at %s: %w", config.TLSKeyFile, err)
		}
		cert, err := tls.X509KeyPair(cert_txt, key_txt)
		if err != nil {
			return nil, fmt.Errorf("error while parsing x509 key pair: %w", err)
		}
		tlsConfig = &tls.Config{
			MinVersion:   tls.VersionTLS10,
			MaxVersion:   tls.VersionTLS12,
			Certificates: []tls.Certificate{cert},
			ServerName:   config.TLSServerName,
		}
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
		tlsConfig:  tlsConfig,
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
		return fmt.Errorf("error while checking for root object existance: %w", err)
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
		ATTR_OBJECTCLASS:        store.AttributeValue{"top", "dcObject", "organization"},
		"structuralobjectclass": store.AttributeValue{"organization"},
		ATTR_CREATORSNAME:       store.AttributeValue{server.baseDN.String()},
		ATTR_CREATETIMESTAMP:    store.AttributeValue{genTimestamp()},
		ATTR_ENTRYUUID:          store.AttributeValue{uuid.String()},
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

func (server *Bottin) TLS() *tls.Config {
	return server.tlsConfig
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

	return nil, fmt.Errorf("DN %s is not under baseDN (%s), and should not be extended for this operation", dn.String(), baseDN.String())
}

func (server *Bottin) HandleStartTLS(ctx context.Context, w ldapserver.ResponseWriter, m *ldapserver.Message) context.Context {
	tlsConn := tls.Server(m.Client.GetConn(), server.tlsConfig)
	res := ldapserver.NewExtendedResponse(ldap.ResultCodeSuccess)
	res.SetResponseName(ldapserver.NoticeOfStartTLS)
	w.Write(res)

	if err := tlsConn.Handshake(); err != nil {
		server.logger.WarnContext(ctx, "error while performing StartTLS", "err", err)

		res.SetDiagnosticMessage(fmt.Sprintf("StartTLS Handshake error : \"%s\"", err.Error()))
		res.SetResultCode(ldap.ResultCodeOperationsError)
		w.Write(res)
		return ctx
	}

	m.Client.SetConn(tlsConn)
	return ctx
}

func (server *Bottin) HandlePasswordModify(ctx context.Context, w ldapserver.ResponseWriter, m *ldapserver.Message) context.Context {
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

func (server *Bottin) handlePasswordModifyInternal(ctx context.Context, r *ldap.ExtendedRequest) (int, error) {
	passwordModifyRequest, err := r.PasswordModifyRequest()
	if err != nil {
		return ldap.ResultCodeInvalidAttributeSyntax, fmt.Errorf("error while parsing PasswordModify: %w", err)
	}

	if passwordModifyRequest.NewPassword() == nil {
		return ldap.ResultCodeAuthMethodNotSupported, errors.New("new password is missing")
	}
	passwd := passwordModifyRequest.NewPassword()

	user := ldapserver.GetUser[User](ctx, EmptyUser)
	if user.user == AnonymousUser {
		return ldap.ResultCodeInsufficientAccessRights, errors.New("not logged in")
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
	if !server.acl.Check(user, "modify", dn, []store.AttributeKey{ATTR_USERPASSWORD}) {
		return ldap.ResultCodeInsufficientAccessRights, fmt.Errorf("Insufficient access rights for %#v", user)
	}
	if dn.Equal(server.baseDN) {
		return ldap.ResultCodeInvalidDNSyntax, errors.New("root entry password cannot be set")
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
	attrs[ATTR_USERPASSWORD] = store.AttributeValue{hash}
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

func (server *Bottin) HandleBind(ctx context.Context, w ldapserver.ResponseWriter, m *ldapserver.Message) context.Context {
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

func (server *Bottin) handleBindInternal(ctx context.Context, r *ldap.BindRequest) (context.Context, int, error) {
	user := ldapserver.GetUser[User](ctx, EmptyUser)

	dn, err := server.parseDN(string(r.Name()), false)
	if err != nil {
		return nil, ldap.ResultCodeInvalidDNSyntax, err
	}

	server.logger.InfoContext(ctx, "bind attempt", "dn", dn)

	// Check permissions
	if !server.acl.Check(user, "bind", dn, []store.AttributeKey{}) {
		return ctx, ldap.ResultCodeInsufficientAccessRights, fmt.Errorf("Insufficient access rights for %#v", user)
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
		hashes = entry.Get(ATTR_USERPASSWORD)
	}

	for _, hash := range hashes {
		server.logger.InfoContext(ctx, "matching against hash", "hash", hash)
		valid, err := matchPassword(hash, passwd)
		if err != nil {
			return ctx, ldap.ResultCodeInvalidCredentials, fmt.Errorf("can't authenticate: %w", err)
		}

		if valid {
			groups := entry.Get(ATTR_MEMBEROF)
			ctx = ldapserver.WithUser(ctx, User{
				user:   string(r.Name()),
				groups: groups,
			})

			return ctx, ldap.ResultCodeSuccess, nil
		}
	}

	return ctx, ldap.ResultCodeInvalidCredentials, fmt.Errorf("No password match")
}
