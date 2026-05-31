package s3

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	_ "embed"

	"github.com/hack-pad/hackpadfs"
	_ "github.com/lib/pq"
)

//go:embed schema.sql
var schema string

type metadata struct {
	db *sql.DB
}

func newMetadata(url string) (*metadata, error) {
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

	return &metadata{db: db}, nil
}

type inode struct {
	metadata *metadata

	id     uint64
	parent uint64
	name   string

	size     uint64
	modified time.Time
	mode     uint64
}

var getQuery = `
WITH RECURSIVE walk AS (
    SELECT id, parent_id, name, 0 AS depth
    FROM inodes
    WHERE parent_id IS NULL
      AND name = $1[1]

    UNION ALL

    SELECT i.id, i.parent_id, i.name, w.depth + 1
    FROM inodes i
    JOIN walk w ON i.parent_id = w.id
    WHERE i.name = $1[w.depth + 2]
)
SELECT id, parent_id, name, size, modified, mode
FROM walk
WHERE depth = array_length($1, 1) - 1;
`

func (m *metadata) get(ctx context.Context, path string) (node *inode, err error) {
	row := m.db.QueryRowContext(ctx, getQuery, filepath.SplitList(path))

	if err := row.Scan(&node.id, &node.parent, &node.name, &node.size, &node.modified, &node.mode); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("(%w) could not find inode at %q: %w", hackpadfs.ErrNotExist, path, err)
		}

		return nil, fmt.Errorf("could not extract six columns from psql inode query: %w", err)
	}

	node.metadata = m
	return node, nil
}

var listQuery = `
SELECT id, parent_id, name, size, modified, mode
FROM inodes
WHERE parent_id = $1
`

func (node *inode) list(ctx context.Context) (inodes []*inode, err error) {
	rows, err := node.metadata.db.QueryContext(ctx, listQuery, node.id)
	if err != nil {
		return nil, fmt.Errorf("error while listing children of inode %d psql: %w", node.id, err)
	}

	defer func() {
		if rowsErr := rows.Close(); rowsErr != nil && err == nil {
			err = fmt.Errorf("error while closing psql rows iterator: %w", rowsErr)
		}
	}()

	for rows.Next() {
		var node inode

		if err := rows.Scan(&node.id, &node.parent, &node.name, &node.size, &node.modified, &node.mode); err != nil {
			return nil, fmt.Errorf("could not extract six columns from psql inode list query: %w", err)
		}

		inodes = append(inodes, &node)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("could not read all psql results: %w", err)
	}

	return inodes, nil
}
