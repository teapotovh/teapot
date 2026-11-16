package mail

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/teapotovh/teapot/lib/ldap"
)

const recipientCacheTTL = 5 * time.Minute

var (
	// ErrRecipientNotFound is returned when a recipient does not exist.
	ErrRecipientNotFound = errors.New("recipient not found")

	// ErrRecipientTemporary is returned when a recipient lookup failed temporarily.
	ErrRecipientTemporary = errors.New("temporary recipient lookup failure")
)

// RecipientResolver resolves a recipient address to a mailbox owner.
type RecipientResolver interface {
	ResolveRecipient(ctx context.Context, address string) (owner string, err error)
}

type recipientCacheEntry struct {
	owner     string
	expiresAt time.Time
}

type LDAPResolver struct {
	logger  *slog.Logger
	factory *ldap.Factory

	mu    sync.RWMutex
	cache map[string]recipientCacheEntry
}

func NewLDAPResolver(factory *ldap.Factory, logger *slog.Logger) *LDAPResolver {
	return &LDAPResolver{
		logger:  logger,
		factory: factory,
		cache:   map[string]recipientCacheEntry{},
	}
}

func (r *LDAPResolver) ResolveRecipient(ctx context.Context, address string) (string, error) {
	address = strings.ToLower(strings.TrimSpace(address))
	if address == "" {
		return "", ErrRecipientNotFound
	}

	r.mu.RLock()
	entry, ok := r.cache[address]
	r.mu.RUnlock()

	if ok && time.Now().Before(entry.expiresAt) {
		return entry.owner, nil
	}

	client, err := r.factory.NewClient(ctx)
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrRecipientTemporary, err)
	}
	defer client.Close()

	users, err := client.Users(ctx)
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrRecipientTemporary, err)
	}

	for _, user := range users {
		if strings.EqualFold(user.Mail, address) {
			r.mu.Lock()
			r.cache[address] = recipientCacheEntry{owner: user.Username, expiresAt: time.Now().Add(recipientCacheTTL)}
			r.mu.Unlock()

			return user.Username, nil
		}
	}

	return "", ErrRecipientNotFound
}
