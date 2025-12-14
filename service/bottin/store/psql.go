package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	_ "embed"
	_ "github.com/lib/pq"
)

//go:embed schema.sql
var schema string

type PSQL struct {
	db *sql.DB
}

func NewPSQL(url string) (*PSQL, error) {
	db, err := sql.Open("postgres", url)
	if err != nil {
		return nil, fmt.Errorf("error while opening connection to psql: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("error while connecting to psql: %w", err)
	}

	tx, err := db.Begin()
	if err != nil {
		return nil, fmt.Errorf("error while beginning migration: %w", err)
	}

	if _, err := tx.Exec(schema); err != nil {
		return nil, fmt.Errorf("error while applying schema: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("error while committing migration: %w", err)
	}

	return &PSQL{db: db}, nil
}

var subListQuery = `
		SELECT dn, attributes
		FROM entries
		WHERE dn LIKE $1;
	`

var exactListQuery = `
		SELECT dn, attributes
		FROM entries
		WHERE dn = $1;
	`

func (p *PSQL) List(ctx context.Context, prefix Prefix, exact bool) (entries []Entry, er error) {
	var query string

	prfx := prefix.String()

	if exact {
		query = exactListQuery
	} else {
		query = subListQuery
		prfx += "%"
	}

	rows, err := p.db.QueryContext(ctx, query, prfx)
	if err != nil {
		return nil, fmt.Errorf("error while listing resources from psql: %w", err)
	}

	defer func() {
		if e := rows.Close(); e != nil {
			err = fmt.Errorf("error while closing psql rows iterator: %w", e)
		}
	}()

	for rows.Next() {
		var (
			rawPrefix     string
			rawAttributes []byte
		)

		if err := rows.Scan(&rawPrefix, &rawAttributes); err != nil {
			return nil, fmt.Errorf("could not extract two columns from psql list: %w", err)
		}

		var attributes Attributes
		if err := json.Unmarshal(rawAttributes, &attributes); err != nil {
			return nil, fmt.Errorf("error while decoding JSON attributes field: %w", err)
		}

		prefix, err := ParsePrefix(rawPrefix)
		if err != nil {
			return nil, fmt.Errorf("error while decoding entry prefix: %w", err)
		}

		entries = append(entries, Entry{
			DN:         prefix.DN(),
			Attributes: attributes,
		})
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("could not read all psql results: %w", err)
	}

	return entries, nil
}

func (p *PSQL) Begin(ctx context.Context) (Transaction, error) {
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("could not begin psql transaction: %w", err)
	}

	return &PSQLTransaction{ctx: ctx, tx: tx}, nil
}

type PSQLTransaction struct {
	ctx context.Context
	tx  *sql.Tx
}

func (p *PSQLTransaction) Context() context.Context {
	return p.ctx
}

var storeQuery = `
		INSERT INTO entries (dn, attributes)
		VALUES ($1, $2::jsonb)
		ON CONFLICT (dn) DO UPDATE
		SET attributes = EXCLUDED.attributes;
	`

func (p *PSQLTransaction) Store(entry Entry) error {
	rawAttributes, err := json.Marshal(entry.Attributes)
	if err != nil {
		return fmt.Errorf("could not marshal entry attributes for psql: %w", err)
	}

	prefix := entry.DN.Prefix().String()

	_, err = p.tx.Exec(storeQuery, prefix, rawAttributes)
	if err != nil {
		return fmt.Errorf("error while inserting data with psql: %w", err)
	}

	return nil
}

var deleteQuery = `DELETE FROM entries WHERE dn = $1;`

func (p *PSQLTransaction) Delete(dn DN) error {
	prefix := dn.Prefix().String()

	_, err := p.tx.Exec(deleteQuery, prefix)
	if err != nil {
		return fmt.Errorf("error while deleting entry in psql: %w", err)
	}

	return nil
}

func (p *PSQLTransaction) Commit() error {
	if err := p.tx.Commit(); err != nil {
		return fmt.Errorf("could not commit psql transaction: %w", err)
	}

	return nil
}
