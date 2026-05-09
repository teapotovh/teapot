package bottin

import (
	"context"
	"errors"
	"fmt"
	"maps"

	"github.com/teapotovh/teapot/lib/ldapsrv"
	ldap "github.com/teapotovh/teapot/lib/ldapsrv/goldap"
	"github.com/teapotovh/teapot/service/bottin/store"
)

var ErrUnsupportedExtendedRequest = errors.New("unsupported extended request, we only support PasswordModify")

func (server *Bottin) Bind(ctx context.Context, r ldap.BindRequest) (context.Context, error) {
	user := ldapsrv.GetUser(ctx, EmptyUser)

	dn, err := server.parseDN(string(r.Name()), false)
	if err != nil {
		return ctx, fmt.Errorf("(%w) %w", ldapsrv.ErrInvalidDNSyntax, err)
	}

	server.logger.InfoContext(ctx, "bind attempt", "dn", dn, "user", user)

	// Check permissions
	if !server.acl.Check(user, "bind", dn, []store.AttributeKey{}) {
		return ctx, fmt.Errorf(
			"could not authentiate as %q: %w",
			dn,
			ldapsrv.ErrInsufficientAccessRights,
		)
	}

	entry, err := server.getEntry(ctx, dn)
	if err != nil {
		return ctx, err
	}

	passwd := string(r.AuthenticationSimple())

	var hashes store.AttributeValue
	if dn.Equal(server.baseDN) {
		hashes = store.AttributeValue{server.rootPasswd}
	} else {
		hashes = entry.Get(AttrUserPassword)
	}

	var errs []error
	for _, hash := range hashes {
		server.logger.InfoContext(ctx, "matching against hash", "hash", hash)

		valid, err := matchPassword(hash, passwd)
		if err != nil {
			errs = append(errs, fmt.Errorf("couldn't authenticate: %w", err))
		}

		if valid {
			groups := entry.Get(AttrMemberOf)
			ctx = ldapsrv.WithUser(ctx, User{
				user:   string(r.Name()),
				groups: groups,
			})

			return ctx, nil
		}
	}

	return ctx, fmt.Errorf("(%w) %w", ldapsrv.ErrInvalidCredentials, errors.Join(errs...))
}

func (server *Bottin) Extended(ctx context.Context, r ldap.ExtendedRequest) error {
	if r.RequestName() != ldapsrv.NoticeOfPasswordModify {
		return fmt.Errorf("(%w) %w", ldapsrv.ErrUnwillingToPerform, ErrUnsupportedExtendedRequest)
	}

	passwordModifyRequest, err := r.PasswordModifyRequest()
	if err != nil {
		return fmt.Errorf("(%w) error while parsing PasswordModify: %w", ldapsrv.ErrInvalidAttributeSyntax, err)
	}

	if passwordModifyRequest.NewPassword() == nil {
		return fmt.Errorf("(%w) %w", ldapsrv.ErrAuthMethodNotSupported, ErrMissingNewPassword)
	}

	passwd := passwordModifyRequest.NewPassword()
	user := ldapsrv.GetUser(ctx, EmptyUser)
	if user.user == AnonymousUser {
		return fmt.Errorf("(%w) %w", ldapsrv.ErrInsufficientAccessRights, ErrNotAuthenticated)
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
		return fmt.Errorf("(%w) %w", ldapsrv.ErrInvalidDNSyntax, err)
	}

	// Check permissions
	if !server.acl.Check(user, "modify", dn, []store.AttributeKey{AttrUserPassword}) {
		return fmt.Errorf(
			"could not modify password for %q: %w",
			dn,
			ldapsrv.ErrInsufficientAccessRights,
		)
	}

	if dn.Equal(server.baseDN) {
		return fmt.Errorf("(%w) %w", ldapsrv.ErrInvalidDNSyntax, ErrSetRootPasswd)
	}

	entry, err := server.getEntry(ctx, dn)
	if err != nil {
		return err
	}

	hash, err := ssha512Encode(string(*passwd))
	if err != nil {
		return fmt.Errorf("(%w) error while hashing passwd: %w", ldapsrv.ErrOperationsError, err)
	}

	server.logger.InfoContext(ctx, "updating passwd", "dn", dn, "hash", hash)

	attrs := maps.Clone(entry.Attributes)
	attrs[AttrUserPassword] = store.AttributeValue{hash}

	tx, err := server.store.Begin(ctx)
	if err != nil {
		return fmt.Errorf("(%w) error while beginning transaction: %w", ldapsrv.ErrOperationsError, err)
	}

	if err = tx.Store(store.NewEntry(dn, attrs)); err != nil {
		return fmt.Errorf("(%w) error while updating password: %w", ldapsrv.ErrOperationsError, err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("(%w) could not commit transaction: %w", ldapsrv.ErrOperationsError, err)
	}

	return nil
}
