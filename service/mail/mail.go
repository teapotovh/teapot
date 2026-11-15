package mail

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"

	"github.com/teapotovh/teapot/lib/ldap"
	"github.com/teapotovh/teapot/service/mail/store"
)

type Config struct {
	Store store.StoreConfig
	LDAP  ldap.LDAPConfig

	Authenticator ldap.Authenticator
	Resolver      RecipientResolver
	StoreInstance store.Store
}

type Mail struct {
	logger *slog.Logger

	ldapFactory   *ldap.Factory
	store         store.Store
	notifications *Notifications
	authenticator ldap.Authenticator
	resolver      RecipientResolver
}

func NewMail(config Config, logger *slog.Logger) (*Mail, error) {
	notifications, err := NewNotifications(notificationURL(config.Store), logger.With("component", "notifications"))
	if err != nil {
		return nil, fmt.Errorf("error while building the notification manager: %w", err)
	}

	config.Store.Notifier = notifications

	var ldapFactory *ldap.Factory

	if config.Authenticator == nil || config.Resolver == nil {
		ldapFactory, err = ldap.NewFactory(config.LDAP, logger.With("component", "ldap"))
		if err != nil {
			return nil, fmt.Errorf("error while building the LDAP factory: %w", err)
		}
	}

	mailStore := config.StoreInstance
	if mailStore == nil {
		mailStore, err = store.NewStore(config.Store, logger.With("component", "store"))
		if err != nil {
			return nil, fmt.Errorf("error while building the mail store: %w", err)
		}
	}

	if setter, ok := mailStore.(interface{ SetNotifier(notifier store.Notifier) }); ok {
		setter.SetNotifier(notifications)
	}

	authenticator := config.Authenticator
	if authenticator == nil {
		authenticator = ldapFactory
	}

	resolver := config.Resolver
	if resolver == nil {
		resolver = NewLDAPResolver(ldapFactory, logger.With("component", "recipient"))
	}

	return &Mail{
		logger:        logger,
		ldapFactory:   ldapFactory,
		store:         mailStore,
		notifications: notifications,
		authenticator: authenticator,
		resolver:      resolver,
	}, nil
}

func (m *Mail) Store() store.Store {
	return m.store
}

func (m *Mail) LDAPFactory() *ldap.Factory {
	return m.ldapFactory
}

func (m *Mail) Authenticator() ldap.Authenticator {
	return m.authenticator
}

func (m *Mail) Resolver() RecipientResolver {
	return m.resolver
}

func (m *Mail) Notifications() *Notifications {
	return m.notifications
}

func notificationURL(config store.StoreConfig) string {
	if config.Type == "online" {
		return config.URL
	}

	return ""
}

func (m *Mail) Subscribe() (<-chan uuid.UUID, func()) {
	return m.notifications.Subscribe()
}

func (m *Mail) ResolveRecipient(ctx context.Context, address string) (string, error) {
	return m.resolver.ResolveRecipient(ctx, address)
}
