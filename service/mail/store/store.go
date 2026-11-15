package store

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/teapotovh/teapot/lib/observability"
	"github.com/teapotovh/teapot/lib/run"
	"github.com/teapotovh/teapot/lib/s3cache"
)

type Store interface {
	run.Runnable
	observability.Metrics
	observability.ReadinessChecks

	Ping(ctx context.Context) error

	ListMailboxes(ctx context.Context, owner string) ([]Mailbox, error)
	GetMailbox(ctx context.Context, owner, name string) (*Mailbox, error)
	CreateMailbox(ctx context.Context, owner, name string, specialUse ...string) (*Mailbox, error)
	DeleteMailbox(ctx context.Context, owner, name string) error
	RenameMailbox(ctx context.Context, owner, oldName, newName string) error
	SetSubscribed(ctx context.Context, owner, name string, subscribed bool) error

	ListMessages(ctx context.Context, mailboxID uuid.UUID) ([]Message, error)
	GetMessages(ctx context.Context, mailboxID uuid.UUID, uids []uint32) ([]Message, error)
	Body(ctx context.Context, message Message) ([]byte, error)

	Append(
		ctx context.Context,
		mailboxID uuid.UUID,
		data []byte,
		flags []string,
		internalDate time.Time,
	) (*Message, error)
	StoreFlags(ctx context.Context, mailboxID uuid.UUID, uids []uint32, op FlagOp, flags []string) ([]Message, error)
	Expunge(ctx context.Context, mailboxID uuid.UUID, uids []uint32) error
	Copy(ctx context.Context, sourceID, destinationID uuid.UUID, uids []uint32) ([]uint32, []uint32, error)
	Move(ctx context.Context, sourceID, destinationID uuid.UUID, uids []uint32) ([]uint32, []uint32, error)
}

// Notifier publishes mailbox changes.
type Notifier interface {
	Notify(ctx context.Context, tx pgx.Tx, mailboxID uuid.UUID) error
	Publish(mailboxID uuid.UUID)
}

type FlagOp int

const (
	FlagSet FlagOp = iota
	FlagAdd
	FlagDel
)

type StoreConfig struct {
	Timeout  time.Duration
	Type     string
	URL      string
	S3       S3Config
	Notifier Notifier
}

type S3Config struct {
	URL    string
	Region string
	Bucket string
	Cache  s3cache.S3CacheConfig
}

func NewStore(config StoreConfig, logger *slog.Logger) (Store, error) {
	ctx, cancel := context.WithTimeout(context.Background(), config.Timeout)
	defer cancel()

	switch config.Type {
	case "mem":
		return NewMem(config.Notifier), nil
	case "online":
		return NewOnline(ctx, config, logger)
	default:
		return nil, fmt.Errorf("error instantiating store of type %q: %w", config.Type, ErrInvalidBackend)
	}
}

var ErrNoBody = errors.New("message body is not available")
