package db

import "database/sql"

// DB returns the underlying *sql.DB if the DBTX is a *sql.DB.
// This is needed for raw queries like FTS5 that sqlc doesn't support.
func (q *Queries) DB() *sql.DB {
	if db, ok := q.db.(*sql.DB); ok {
		return db
	}
	return nil
}
