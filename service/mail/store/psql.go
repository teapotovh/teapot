package store

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"git.sr.ht/~bitfehler/brant"
	"git.sr.ht/~bitfehler/brant/database/dialect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/teapotovh/teapot/lib/observability"
	"github.com/teapotovh/teapot/lib/run"

	// Register the PostgreSQL driver used by the migration provider.
	_ "github.com/jackc/pgx/v5/stdlib"
)

//go:embed migrations/*.sql
var migrations embed.FS

type Online struct {
	logger   *slog.Logger
	pool     *pgxpool.Pool
	blobs    blobStore
	metrics  metrics
	notifier Notifier
}

func NewOnline(ctx context.Context, config StoreConfig, logger *slog.Logger) (*Online, error) {
	options := brant.DefaultOptions().
		WithTableName("_mail_version").
		WithFilesystem(migrations).
		WithDataSourceName(config.URL)

	provider, err := brant.NewProvider(logger, dialect.Postgres, options)
	if err != nil {
		return nil, fmt.Errorf("error while constructing the migration provider: %w", err)
	}

	applied, err := provider.Up(ctx)
	if err != nil {
		return nil, fmt.Errorf("error while applying migrations: %w", err)
	}

	for _, migration := range applied {
		logger.Info("applied migration", "migration", migration)
	}

	if err := provider.Close(); err != nil {
		return nil, fmt.Errorf("error while closing the migration connection: %w", err)
	}

	poolConfig, err := pgxpool.ParseConfig(config.URL)
	if err != nil {
		return nil, fmt.Errorf("error while parsing the psql URL: %w", err)
	}

	poolConfig.MaxConns = 8
	poolConfig.MinConns = 1

	pool, err := pgxpool.NewWithConfig(context.Background(), poolConfig)
	if err != nil {
		return nil, fmt.Errorf("error while opening the psql pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("error while connecting to psql: %w", err)
	}

	blobs, err := newS3Blobs(ctx, config.S3, logger.With("component", "s3"))
	if err != nil {
		pool.Close()
		return nil, err
	}

	online := &Online{
		logger:   logger,
		pool:     pool,
		blobs:    blobs,
		notifier: config.Notifier,
	}
	online.metrics.initMetrics("online")

	return online, nil
}

func scanMailbox(row interface {
	Scan(dest ...any) error
}) (*Mailbox, error) {
	var (
		box         Mailbox
		uidValidity int64
		uidNext     int64
		highestMod  int64
	)

	if err := row.Scan(
		&box.ID,
		&box.Owner,
		&box.Name,
		&uidValidity,
		&uidNext,
		&highestMod,
		&box.ChangeSeq,
		&box.Subscribed,
		&box.SpecialUse,
		&box.CreatedAt,
	); err != nil {
		return nil, err
	}

	box.UIDValidity = uint32(uidValidity)  //nolint:gosec
	box.UIDNext = uint32(uidNext)          //nolint:gosec
	box.HighestModSeq = uint64(highestMod) //nolint:gosec

	return &box, nil
}

const mailboxColumns = `id, owner, name, uid_validity, uid_next, highest_modseq, change_seq, subscribed, special_use, created_at`

func (o *Online) Ping(ctx context.Context) error {
	return o.pool.Ping(ctx)
}

//nolint:funcorder
func (o *Online) notifyMailbox(ctx context.Context, tx pgx.Tx, mailboxID uuid.UUID) error {
	if o.notifier == nil {
		return nil
	}

	return o.notifier.Notify(ctx, tx, mailboxID)
}

func (o *Online) ListMailboxes(ctx context.Context, owner string) ([]Mailbox, error) {
	rows, err := o.pool.Query(ctx,
		`SELECT `+mailboxColumns+` FROM mailboxes WHERE owner = $1 AND deleted_at IS NULL ORDER BY name`,
		owner,
	)
	if err != nil {
		return nil, fmt.Errorf("error while listing mailboxes: %w", err)
	}
	defer rows.Close()

	var result []Mailbox

	for rows.Next() {
		box, err := scanMailbox(rows)
		if err != nil {
			return nil, fmt.Errorf("error while scanning mailbox: %w", err)
		}

		result = append(result, *box)
	}

	return result, rows.Err()
}

func (o *Online) GetMailbox(ctx context.Context, owner, name string) (*Mailbox, error) {
	box, err := scanMailbox(o.pool.QueryRow(ctx,
		`SELECT `+mailboxColumns+` FROM mailboxes WHERE owner = $1 AND name = $2 AND deleted_at IS NULL`,
		owner, CanonicalMailboxName(name),
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrMailboxNotFound
		}

		return nil, fmt.Errorf("error while getting mailbox: %w", err)
	}

	return box, nil
}

func (o *Online) CreateMailbox(ctx context.Context, owner, name string, specialUse ...string) (*Mailbox, error) {
	name = CanonicalMailboxName(name)
	if !ValidMailboxName(name) {
		return nil, fmt.Errorf("%w: %q", ErrInvalidMailboxName, name)
	}

	id, err := uuid.NewRandom()
	if err != nil {
		return nil, fmt.Errorf("error while generating mailbox id: %w", err)
	}

	if specialUse == nil {
		specialUse = []string{}
	}

	box, err := scanMailbox(o.pool.QueryRow(ctx,
		`INSERT INTO mailboxes (id, owner, name, uid_validity, uid_next, highest_modseq, special_use)
		 VALUES ($1, $2, $3, $4, 1, 0, $5)
		 RETURNING `+mailboxColumns,
		id, owner, name, randomUIDValidity(), specialUse,
	))
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrMailboxAlreadyExist
		}

		return nil, fmt.Errorf("error while creating mailbox: %w", err)
	}

	return box, nil
}

func (o *Online) DeleteMailbox(ctx context.Context, owner, name string) error {
	name = CanonicalMailboxName(name)

	tx, err := o.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("error while beginning transaction: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	box, err := scanMailbox(tx.QueryRow(ctx,
		`SELECT `+mailboxColumns+` FROM mailboxes WHERE owner = $1 AND name = $2 AND deleted_at IS NULL FOR UPDATE`,
		owner, name,
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrMailboxNotFound
		}

		return fmt.Errorf("error while getting mailbox: %w", err)
	}

	var children int
	if err := tx.QueryRow(ctx,
		`SELECT count(*) FROM mailboxes
		 WHERE owner = $1 AND deleted_at IS NULL AND left(name, length($2) + 1) = $2 || '/'`,
		owner, box.Name,
	).Scan(&children); err != nil {
		return fmt.Errorf("error while counting children: %w", err)
	}

	if children > 0 {
		return ErrMailboxHasChildren
	}

	if _, err := tx.Exec(ctx, `UPDATE mailboxes SET deleted_at = now() WHERE id = $1`, box.ID); err != nil {
		return fmt.Errorf("error while deleting mailbox: %w", err)
	}

	if err := o.notifyMailbox(ctx, tx, box.ID); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (o *Online) RenameMailbox(ctx context.Context, owner, oldName, newName string) error {
	oldName = CanonicalMailboxName(oldName)

	newName = CanonicalMailboxName(newName)
	if !ValidMailboxName(newName) {
		return fmt.Errorf("%w: %q", ErrInvalidMailboxName, newName)
	}

	tx, err := o.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("error while beginning transaction: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	box, err := scanMailbox(tx.QueryRow(ctx,
		`SELECT `+mailboxColumns+` FROM mailboxes WHERE owner = $1 AND name = $2 AND deleted_at IS NULL FOR UPDATE`,
		owner, oldName,
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrMailboxNotFound
		}

		return fmt.Errorf("error while getting mailbox: %w", err)
	}

	if _, err := tx.Exec(ctx,
		`UPDATE mailboxes
		 SET name = $3 || substring(name from length($2) + 1)
		 WHERE owner = $1 AND deleted_at IS NULL
		   AND (name = $2 OR left(name, length($2) + 1) = $2 || '/')`,
		owner, box.Name, newName,
	); err != nil {
		if isUniqueViolation(err) {
			return ErrMailboxAlreadyExist
		}

		return fmt.Errorf("error while renaming mailbox: %w", err)
	}

	if err := o.notifyMailbox(ctx, tx, box.ID); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (o *Online) SetSubscribed(ctx context.Context, owner, name string, subscribed bool) error {
	tag, err := o.pool.Exec(ctx,
		`UPDATE mailboxes SET subscribed = $3 WHERE owner = $1 AND name = $2 AND deleted_at IS NULL`,
		owner, CanonicalMailboxName(name), subscribed,
	)
	if err != nil {
		return fmt.Errorf("error while updating subscription: %w", err)
	}

	if tag.RowsAffected() == 0 {
		return ErrMailboxNotFound
	}

	return nil
}

const messageColumns = `mailbox_id, uid, blob_sha256, flags, internal_date, size, modseq`

func scanMessage(row interface {
	Scan(dest ...any) error
}) (*Message, error) {
	var (
		msg Message
		uid int64
		mod int64
	)

	if err := row.Scan(&msg.MailboxID, &uid, &msg.Digest, &msg.Flags, &msg.InternalDate, &msg.Size, &mod); err != nil {
		return nil, err
	}

	msg.UID = uint32(uid)    //nolint:gosec
	msg.ModSeq = uint64(mod) //nolint:gosec
	msg.Flags = NormalizeFlags(msg.Flags)

	return &msg, nil
}

func (o *Online) ListMessages(ctx context.Context, mailboxID uuid.UUID) ([]Message, error) {
	rows, err := o.pool.Query(ctx,
		`SELECT `+messageColumns+` FROM messages
		 WHERE mailbox_id = $1 AND expunged_at IS NULL ORDER BY uid`,
		mailboxID,
	)
	if err != nil {
		return nil, fmt.Errorf("error while listing messages: %w", err)
	}
	defer rows.Close()

	var result []Message

	for rows.Next() {
		msg, err := scanMessage(rows)
		if err != nil {
			return nil, fmt.Errorf("error while scanning message: %w", err)
		}

		result = append(result, *msg)
	}

	return result, rows.Err()
}

func (o *Online) GetMessages(ctx context.Context, mailboxID uuid.UUID, uids []uint32) ([]Message, error) {
	rows, err := o.pool.Query(ctx,
		`SELECT `+messageColumns+` FROM messages
		 WHERE mailbox_id = $1 AND uid = ANY($2) AND expunged_at IS NULL ORDER BY uid`,
		mailboxID, uids,
	)
	if err != nil {
		return nil, fmt.Errorf("error while getting messages: %w", err)
	}
	defer rows.Close()

	var result []Message

	for rows.Next() {
		msg, err := scanMessage(rows)
		if err != nil {
			return nil, fmt.Errorf("error while scanning message: %w", err)
		}

		result = append(result, *msg)
	}

	return result, rows.Err()
}

func (o *Online) Body(ctx context.Context, message Message) ([]byte, error) {
	return o.blobs.Get(ctx, message.Digest)
}

//nolint:funcorder
func (o *Online) allocateUIDs(
	ctx context.Context,
	tx pgx.Tx,
	mailboxID uuid.UUID,
	count int,
) (uint32, int64, uint64, error) {
	var (
		firstUID   int64
		changeSeq  int64
		highestMod int64
	)

	if err := tx.QueryRow(ctx,
		`UPDATE mailboxes
		 SET uid_next = uid_next + $2,
		     change_seq = change_seq + 1,
		     highest_modseq = highest_modseq + 1
		 WHERE id = $1 AND deleted_at IS NULL
		 RETURNING uid_next - $2, change_seq, highest_modseq`,
		mailboxID, count,
	).Scan(&firstUID, &changeSeq, &highestMod); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, 0, 0, ErrMailboxNotFound
		}

		return 0, 0, 0, fmt.Errorf("error while allocating UIDs: %w", err)
	}

	return uint32(firstUID), changeSeq, uint64(highestMod), nil //nolint:gosec
}

func (o *Online) Append(
	ctx context.Context,
	mailboxID uuid.UUID,
	data []byte,
	flags []string,
	internalDate time.Time,
) (*Message, error) {
	digest, err := o.blobs.Put(ctx, data)
	if err != nil {
		return nil, err
	}

	if internalDate.IsZero() {
		internalDate = time.Now().UTC()
	}

	tx, err := o.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("error while beginning transaction: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	if _, err := tx.Exec(ctx,
		`INSERT INTO blobs (sha256, size) VALUES ($1, $2) ON CONFLICT (sha256) DO NOTHING`,
		digest, len(data),
	); err != nil {
		return nil, fmt.Errorf("error while storing blob metadata: %w", err)
	}

	uid, changeSeq, modseq, err := o.allocateUIDs(ctx, tx, mailboxID, 1)
	if err != nil {
		return nil, err
	}

	msg, err := scanMessage(tx.QueryRow(ctx,
		`INSERT INTO messages (mailbox_id, uid, blob_sha256, flags, internal_date, size, modseq, change_seq)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 RETURNING `+messageColumns,
		mailboxID, uid, digest, NormalizeFlags(flags), internalDate.UTC(), len(data), modseq, changeSeq,
	))
	if err != nil {
		return nil, fmt.Errorf("error while inserting message: %w", err)
	}

	if err := o.notifyMailbox(ctx, tx, mailboxID); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("error while committing message: %w", err)
	}

	return msg, nil
}

func (o *Online) StoreFlags(
	ctx context.Context,
	mailboxID uuid.UUID,
	uids []uint32,
	op FlagOp,
	flags []string,
) ([]Message, error) {
	tx, err := o.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("error while beginning transaction: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	_, changeSeq, modseq, err := o.allocateUIDs(ctx, tx, mailboxID, 0)
	if err != nil {
		return nil, err
	}

	rows, err := tx.Query(ctx,
		`SELECT `+messageColumns+` FROM messages
		 WHERE mailbox_id = $1 AND uid = ANY($2) AND expunged_at IS NULL ORDER BY uid FOR UPDATE`,
		mailboxID, uids,
	)
	if err != nil {
		return nil, fmt.Errorf("error while locking messages: %w", err)
	}

	var messages []Message

	for rows.Next() {
		msg, err := scanMessage(rows)
		if err != nil {
			rows.Close()
			return nil, fmt.Errorf("error while scanning message: %w", err)
		}

		messages = append(messages, *msg)
	}

	rows.Close()

	if err := rows.Err(); err != nil {
		return nil, err
	}

	for i := range messages {
		msg := &messages[i]

		switch op {
		case FlagSet:
			msg.Flags = NormalizeFlags(flags)
		case FlagAdd:
			msg.Flags = NormalizeFlags(append(msg.Flags, flags...))
		case FlagDel:
			msg.Flags = removeFlags(msg.Flags, flags)
		}

		msg.ModSeq = modseq

		if _, err := tx.Exec(ctx,
			`UPDATE messages SET flags = $3, modseq = $4, change_seq = $5
			 WHERE mailbox_id = $1 AND uid = $2`,
			mailboxID, msg.UID, msg.Flags, modseq, changeSeq,
		); err != nil {
			return nil, fmt.Errorf("error while updating message flags: %w", err)
		}
	}

	if err := o.notifyMailbox(ctx, tx, mailboxID); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("error while committing flags: %w", err)
	}

	return messages, nil
}

func removeFlags(current, remove []string) []string {
	result := make([]string, 0, len(current))

	for _, flag := range current {
		drop := false

		for _, other := range remove {
			if equalFold(flag, other) {
				drop = true
				break
			}
		}

		if !drop {
			result = append(result, flag)
		}
	}

	return result
}

func (o *Online) Expunge(ctx context.Context, mailboxID uuid.UUID, uids []uint32) error {
	tx, err := o.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("error while beginning transaction: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	_, changeSeq, _, err := o.allocateUIDs(ctx, tx, mailboxID, 0)
	if err != nil {
		return err
	}

	if _, err := tx.Exec(ctx,
		`UPDATE messages SET expunged_at = now(), change_seq = $3
		 WHERE mailbox_id = $1 AND uid = ANY($2) AND expunged_at IS NULL`,
		mailboxID, uids, changeSeq,
	); err != nil {
		return fmt.Errorf("error while expunging messages: %w", err)
	}

	if err := o.notifyMailbox(ctx, tx, mailboxID); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func lockMailboxes(ctx context.Context, tx pgx.Tx, ids ...uuid.UUID) error {
	sort.Slice(ids, func(i, j int) bool { return ids[i].String() < ids[j].String() })

	if _, err := tx.Exec(ctx,
		`SELECT id FROM mailboxes WHERE id = ANY($1) ORDER BY id FOR UPDATE`, ids,
	); err != nil {
		return fmt.Errorf("error while locking mailboxes: %w", err)
	}

	return nil
}

//nolint:funcorder
func (o *Online) copyMessages(
	ctx context.Context,
	tx pgx.Tx,
	sourceID, destinationID uuid.UUID,
	uids []uint32,
) ([]uint32, []uint32, error) {
	rows, err := tx.Query(ctx,
		`SELECT `+messageColumns+` FROM messages
		 WHERE mailbox_id = $1 AND uid = ANY($2) AND expunged_at IS NULL ORDER BY uid FOR UPDATE`,
		sourceID, uids,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("error while reading source messages: %w", err)
	}

	var source []Message

	for rows.Next() {
		msg, err := scanMessage(rows)
		if err != nil {
			rows.Close()
			return nil, nil, err
		}

		source = append(source, *msg)
	}

	rows.Close()

	if err := rows.Err(); err != nil {
		return nil, nil, err
	}

	if len(source) == 0 {
		return nil, nil, nil
	}

	firstUID, changeSeq, modseq, err := o.allocateUIDs(ctx, tx, destinationID, len(source))
	if err != nil {
		return nil, nil, err
	}

	var sourceUIDs, destUIDs []uint32

	for i, msg := range source {
		newUID := firstUID + uint32(i)
		if _, err := tx.Exec(ctx,
			`INSERT INTO messages (mailbox_id, uid, blob_sha256, flags, internal_date, size, modseq, change_seq)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
			destinationID, newUID, msg.Digest, msg.Flags, msg.InternalDate, msg.Size, modseq, changeSeq,
		); err != nil {
			return nil, nil, fmt.Errorf("error while copying message: %w", err)
		}

		sourceUIDs = append(sourceUIDs, msg.UID)
		destUIDs = append(destUIDs, newUID)
	}

	return sourceUIDs, destUIDs, nil
}

func (o *Online) Copy(
	ctx context.Context,
	sourceID, destinationID uuid.UUID,
	uids []uint32,
) ([]uint32, []uint32, error) {
	tx, err := o.pool.Begin(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("error while beginning transaction: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	if err := lockMailboxes(ctx, tx, sourceID, destinationID); err != nil {
		return nil, nil, err
	}

	sourceUIDs, destUIDs, err := o.copyMessages(ctx, tx, sourceID, destinationID, uids)
	if err != nil {
		return nil, nil, err
	}

	if err := o.notifyMailbox(ctx, tx, destinationID); err != nil {
		return nil, nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, nil, fmt.Errorf("error while committing copy: %w", err)
	}

	return sourceUIDs, destUIDs, nil
}

func (o *Online) Move(
	ctx context.Context,
	sourceID, destinationID uuid.UUID,
	uids []uint32,
) ([]uint32, []uint32, error) {
	tx, err := o.pool.Begin(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("error while beginning transaction: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	if err := lockMailboxes(ctx, tx, sourceID, destinationID); err != nil {
		return nil, nil, err
	}

	sourceUIDs, destUIDs, err := o.copyMessages(ctx, tx, sourceID, destinationID, uids)
	if err != nil {
		return nil, nil, err
	}

	if len(sourceUIDs) > 0 {
		_, changeSeq, _, err := o.allocateUIDs(ctx, tx, sourceID, 0)
		if err != nil {
			return nil, nil, err
		}

		if _, err := tx.Exec(ctx,
			`UPDATE messages SET expunged_at = now(), change_seq = $3
			 WHERE mailbox_id = $1 AND uid = ANY($2) AND expunged_at IS NULL`,
			sourceID, sourceUIDs, changeSeq,
		); err != nil {
			return nil, nil, fmt.Errorf("error while expunging source messages: %w", err)
		}
	}

	if err := o.notifyMailbox(ctx, tx, sourceID); err != nil {
		return nil, nil, err
	}

	if err := o.notifyMailbox(ctx, tx, destinationID); err != nil {
		return nil, nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, nil, fmt.Errorf("error while committing move: %w", err)
	}

	return sourceUIDs, destUIDs, nil
}

func (o *Online) Run(ctx context.Context, notify run.Notify) error {
	defer o.pool.Close()

	notify.Notify()
	<-ctx.Done()

	return nil
}

func (o *Online) Metrics() []prometheus.Collector {
	return []prometheus.Collector{o.metrics.backend}
}

func (o *Online) ReadinessChecks() map[string]observability.Check {
	return map[string]observability.Check{
		"mail/psql": observability.CheckFunc(o.Ping),
	}
}

func isUniqueViolation(err error) bool {
	var pgErr interface{ SQLState() string }
	if ok := asPgError(err, &pgErr); ok {
		return pgErr.SQLState() == "23505"
	}

	return false
}

func asPgError(err error, target *interface{ SQLState() string }) bool {
	for err != nil {
		if e, ok := err.(interface{ SQLState() string }); ok {
			*target = e
			return true
		}

		type unwrapper interface{ Unwrap() error }

		u, ok := err.(unwrapper)
		if !ok {
			return false
		}

		err = u.Unwrap()
	}

	return false
}

var (
	_ Store                         = (*Online)(nil)
	_ run.Runnable                  = (*Online)(nil)
	_ observability.Metrics         = (*Online)(nil)
	_ observability.ReadinessChecks = (*Online)(nil)
)
