package store

import (
	"context"
	"fmt"
)

var liveRefsQuery = `
		SELECT DISTINCT ref FROM inodes WHERE kind = $1 AND ref <> '';
	`

// LiveRefs returns the distinct blob refs referenced by live file inodes.
func (p *PSQL) LiveRefs(ctx context.Context) ([]string, error) {
	rows, err := p.pool.Query(ctx, liveRefsQuery, fileKind)
	if err != nil {
		return nil, fmt.Errorf("error while reading live refs: %w", err)
	}

	defer rows.Close()

	refs := make([]string, 0)

	for rows.Next() {
		var ref string
		if err := rows.Scan(&ref); err != nil {
			return nil, fmt.Errorf("error while scanning a live ref: %w", err)
		}

		refs = append(refs, ref)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error while reading live refs: %w", err)
	}

	return refs, nil
}
