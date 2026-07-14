package core

import (
	"math"
	"unsafe"

	portin "github.com/bnema/purego-sqlite/internal/ports/in"
	portout "github.com/bnema/purego-sqlite/internal/ports/out"
)

// Compile-time interface check.
var _ portin.DB = (*database)(nil)

// database implements portin.DB using the outbound CAPI.
type database struct {
	capi portout.CAPI
	ptr  unsafe.Pointer // sqlite3* handle
}

// OpenDB opens a SQLite database at the given DSN (file path or ":memory:").
func OpenDB(capi portout.CAPI, dsn string) (*database, error) {
	db := &database{capi: capi}
	// Use sqlite3_open rather than sqlite3_open_v2 because purego maps Go
	// string "" to a non-NULL char* "\0", but sqlite3_open_v2 requires a
	// real NULL pointer for the zVfs parameter to select the default VFS.
	// sqlite3_open opens in read-write + create mode by default, which is
	// the behaviour we want.
	rc := capi.Sqlite3Open(dsn, unsafe.Pointer(&db.ptr))
	if rc != sqliteOK {
		// If open partially succeeded, there might be a handle to extract
		// an error message from, but we close it anyway.
		if db.ptr != nil {
			msg := capi.Sqlite3Errmsg(db.ptr)
			capi.Sqlite3CloseV2(db.ptr)
			return nil, &Error{Code: int(rc), Msg: msg}
		}
		return nil, &Error{Code: int(rc), Msg: "failed to open database"}
	}
	return db, nil
}

// Prepare prepares a SQL statement for execution.
func (db *database) Prepare(sql string) (portin.Stmt, error) {
	if len(sql) > math.MaxInt32 {
		return nil, &Error{Code: 18, Msg: "SQL string too long"} // SQLITE_TOOBIG = 18
	}
	var stmtPtr unsafe.Pointer
	rc := db.capi.Sqlite3PrepareV2(
		db.ptr,
		sql,
		int32(len(sql)),
		unsafe.Pointer(&stmtPtr),
		nil, // pzTail - we don't use multi-statement parsing
	)
	if rc != sqliteOK {
		return nil, db.lastError(rc)
	}
	nParams := db.capi.Sqlite3BindParameterCount(stmtPtr)
	return &statement{
		db:       db,
		ptr:      stmtPtr,
		numInput: int(nParams),
	}, nil
}

// Exec executes a SQL statement that does not return rows.
func (db *database) Exec(sql string, args ...any) (portin.Result, error) {
	if len(args) == 0 {
		// Fast path: use sqlite3_exec for argument-free statements.
		rc := db.capi.Sqlite3Exec(db.ptr, sql, nil, nil, nil)
		if rc != sqliteOK {
			return nil, db.lastError(rc)
		}
		lastID := db.capi.Sqlite3LastInsertRowid(db.ptr)
		changes := db.capi.Sqlite3Changes(db.ptr)
		return &result{lastID: lastID, changes: int64(changes)}, nil
	}

	stmt, err := db.Prepare(sql)
	if err != nil {
		return nil, err
	}
	defer stmt.Close()
	return stmt.Exec(args...)
}

// Query executes a SQL statement that returns rows.
func (db *database) Query(sql string, args ...any) (portin.Rows, error) {
	stmt, err := db.Prepare(sql)
	if err != nil {
		return nil, err
	}
	s := stmt.(*statement)
	if err := s.bind(args); err != nil {
		s.Close()
		return nil, err
	}
	return &rows{
		stmt:      s,
		ownsStmt:  true,
		exhausted: false,
	}, nil
}

// Close closes the database connection.
func (db *database) Close() error {
	if db.ptr == nil {
		return nil
	}
	rc := db.capi.Sqlite3CloseV2(db.ptr)
	if rc != sqliteOK {
		return db.lastError(rc)
	}
	db.ptr = nil
	return nil
}

// lastError creates an Error from the last SQLite error on this connection.
func (db *database) lastError(rc int32) error {
	msg := db.capi.Sqlite3Errmsg(db.ptr)
	return &Error{Code: int(rc), Msg: msg}
}
