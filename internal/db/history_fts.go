package db

import (
	"context"
	"database/sql"
)

// SearchHistoryFTS performs full-text search on history using FTS5.
// sqlc doesn't support FTS5's "table MATCH ?" syntax, so this is manual.
func SearchHistoryFTS(ctx context.Context, db *sql.DB, query string, limit int64) ([]History, error) {
	const q = `SELECT h.* FROM history h WHERE h.id IN (SELECT rowid FROM history_fts WHERE history_fts MATCH ?) ORDER BY h.last_visited DESC LIMIT ?`
	rows, err := db.QueryContext(ctx, q, query, limit)
	if err != nil {
		return nil, err
	}
	items, scanErr := scanHistoryRows(rows)
	closeErr := rows.Close()
	if scanErr != nil {
		return nil, scanErr
	}
	if closeErr != nil {
		return nil, closeErr
	}
	return items, nil
}

// SearchHistoryFTSWithOffset performs paginated full-text search on history.
func SearchHistoryFTSWithOffset(ctx context.Context, db *sql.DB, query string, limit, offset int64) ([]History, error) {
	const q = `SELECT h.* FROM history h WHERE h.id IN (SELECT rowid FROM history_fts WHERE history_fts MATCH ?) ORDER BY h.last_visited DESC LIMIT ? OFFSET ?`
	rows, err := db.QueryContext(ctx, q, query, limit, offset)
	if err != nil {
		return nil, err
	}
	items, scanErr := scanHistoryRows(rows)
	closeErr := rows.Close()
	if scanErr != nil {
		return nil, scanErr
	}
	if closeErr != nil {
		return nil, closeErr
	}
	return items, nil
}

// SearchHistoryFTSRanked performs full-text search with BM25 ranking.
func SearchHistoryFTSRanked(ctx context.Context, db *sql.DB, query string, limit int64) ([]History, error) {
	const q = `SELECT h.* FROM history h INNER JOIN (SELECT rowid, bm25(history_fts) as rank FROM history_fts WHERE history_fts MATCH ? ORDER BY rank LIMIT ?) fts ON h.id = fts.rowid ORDER BY fts.rank`
	rows, err := db.QueryContext(ctx, q, query, limit)
	if err != nil {
		return nil, err
	}
	items, scanErr := scanHistoryRows(rows)
	closeErr := rows.Close()
	if scanErr != nil {
		return nil, scanErr
	}
	if closeErr != nil {
		return nil, closeErr
	}
	return items, nil
}

func scanHistoryRows(rows *sql.Rows) ([]History, error) {
	var items []History
	var scanErr error
	for rows.Next() {
		var i History
		if err := rows.Scan(
			&i.ID,
			&i.Url,
			&i.Title,
			&i.VisitCount,
			&i.LastVisited,
			&i.CreatedAt,
			&i.FaviconUrl,
		); err != nil {
			scanErr = err
			break
		}
		items = append(items, i)
	}
	if scanErr != nil {
		return nil, scanErr
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}
