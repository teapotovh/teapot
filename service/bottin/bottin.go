package bottin

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"uuid"

	"github.com/teapotovh/teapot/lib/ldapsrv"
	"github.com/teapotovh/teapot/lib/run"
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

	// Root DSE attributes.

	AttrNamingContexts       store.AttributeKey = "namingcontexts"
	AttrSupportedLDAPVersion store.AttributeKey = "supportedldapversion"
	AttrSupportedExtension   store.AttributeKey = "supportedextension"

	ldapVersion = "3"
)

var (
	ErrSetRootPasswd      = errors.New("root entry password cannot be set")
	ErrNotAuthenticated   = errors.New("not authenticated")
	ErrMissingNewPassword = errors.New("new password is missing")
	ErrDNNotPrefix        = errors.New("DN is not a prefix of base DN")
)

type BottinConfig struct {
	Store  store.StoreConfig
	BaseDN string
	Passwd string
	ACL    []string
}

type Bottin struct {
	logger *slog.Logger

	baseDN     store.DN
	rootPasswd string
	acl        ACL

	store store.Store
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

	if config.Passwd == "" {
		adminPassBytes := make([]byte, 8)

		_, err := rand.Read(adminPassBytes)
		if err != nil {
			return nil, fmt.Errorf("error while generating random root password: %w", err)
		}

		config.Passwd = base64.RawURLEncoding.EncodeToString(adminPassBytes)
		logger.Info("using randomly generated root password", "passwd", config.Passwd)
	}

	hash, err := ssha512Encode(config.Passwd)
	if err != nil {
		return nil, fmt.Errorf("error while hashing root passwd: %w", err)
	}

	store, err := store.NewStore(config.Store, logger.With("component", "store"))
	if err != nil {
		return nil, fmt.Errorf("error while initializing bottin store: %w", err)
	}

	bottin := Bottin{
		logger: slog.New(ldapsrv.NewContextHandler(logger.Handler())),

		baseDN:     baseDN,
		rootPasswd: hash,
		acl:        acl,

		store: store,
	}

	return &bottin, nil
}

func (server *Bottin) Store() store.Store {
	return server.store
}

func rollbackOnError(ctx context.Context, tx store.Transaction, err *error) {
	if *err == nil {
		return
	}

	*err = errors.Join(*err, tx.Rollback(ctx))
}

type State struct {
	user *User
}

const AnonymousUser = "ANONYMOUS"

func (s State) User() User {
	if s.user != nil {
		return *s.user
	}

	return User{
		user:   AnonymousUser,
		groups: []string{},
	}
}

// Run implements run.Runnable.
func (server *Bottin) Run(step run.Step) error {
	ctx := step.Context()

	if err := server.initialize(ctx); err != nil {
		return fmt.Errorf("error while initializing: %w", err)
	}

	step.Notify()

	return nil
}

func (server *Bottin) initialize(ctx context.Context) (err error) {
	// Check that the root object and the root DSE exist.
	// If they do, we're done. Otherwise, we have some initialization to do.
	baseExists, err := server.existsEntry(ctx, server.baseDN)
	if err != nil {
		return fmt.Errorf("error while checking for root object existence: %w", err)
	}

	rootDSEExists, err := server.existsEntry(ctx, store.DN{})
	if err != nil {
		return fmt.Errorf("error while checking for root DSE existence: %w", err)
	}

	if baseExists && rootDSEExists {
		return nil
	}

	tx, err := server.store.Begin(ctx)
	if err != nil {
		return fmt.Errorf("error while beginning store transaction: %w", err)
	}

	defer rollbackOnError(ctx, tx, &err)

	if !rootDSEExists {
		rootDSEAttributes := store.Attributes{
			AttrObjectClass:          store.AttributeValue{"top"},
			AttrNamingContexts:       store.AttributeValue{server.baseDN.String()},
			AttrSupportedLDAPVersion: store.AttributeValue{ldapVersion},
			AttrSupportedExtension:   store.AttributeValue{string(ldapsrv.NoticeOfPasswordModify)},
		}

		rootDSE := store.NewEntry(store.DN{}, rootDSEAttributes)
		if err = tx.Store(ctx, rootDSE); err != nil {
			return fmt.Errorf("error while storing root DSE: %w", err)
		}
	}

	if !baseExists {
		// We have to initialize the server. Create a root object.
		baseAttributes := store.Attributes{
			AttrObjectClass:         store.AttributeValue{"top", "dcObject", "organization"},
			"structuralobjectclass": store.AttributeValue{"organization"},
			AttrCreatorsName:        store.AttributeValue{server.baseDN.String()},
			AttrCreateTimestamp:     store.AttributeValue{genTimestamp()},
			AttrEntryUUID:           store.AttributeValue{uuid.New().String()},
		}

		entry := store.NewEntry(server.baseDN, baseAttributes)
		if err = tx.Store(ctx, entry); err != nil {
			return fmt.Errorf("error while storing base entry: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("could not commit store transaction: %w", err)
	}

	if !baseExists {
		server.logger.InfoContext(ctx, "initialized base entry", "dn", server.baseDN)
	}

	if !rootDSEExists {
		server.logger.InfoContext(ctx, "initialized root DSE")
	}

	return nil
}

func (server *Bottin) parseDN(rawDN string, allowPrefix bool) (store.DN, error) {
	dn, err := store.ParseDN(rawDN)
	if err != nil {
		return nil, err
	}

	if dn.Level() == 0 && allowPrefix {
		return dn, nil
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
