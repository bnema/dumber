package driver

import "database/sql"

// init registers the purego-sqlite driver as "sqlite3" with database/sql.
// This is the same name used by mattn/go-sqlite3 — do not import both
// in the same binary or sql.Register will panic.
func init() {
	sql.Register("sqlite3", &Driver{})
}
