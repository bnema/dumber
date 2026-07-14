package core

import (
	"fmt"
	"math"
	"unsafe"

	portin "github.com/bnema/purego-sqlite/internal/ports/in"
)

// Compile-time interface check.
var _ portin.Stmt = (*statement)(nil)

// sqliteTransientVal is the SQLITE_TRANSIENT destructor sentinel value:
// (sqlite3_destructor_type)(-1). SQLite treats this as "make a private copy".
// We store the raw uintptr to avoid a go-vet false positive on the conversion
// from integer to unsafe.Pointer (which is intentional for this sentinel).
//
//nolint:gosec
var sqliteTransientVal = ^uintptr(0)

// sqliteTransient returns the SQLITE_TRANSIENT destructor as unsafe.Pointer.
//
//go:nosplit
func sqliteTransient() unsafe.Pointer { return *(*unsafe.Pointer)(unsafe.Pointer(&sqliteTransientVal)) }

// statement implements portin.Stmt.
type statement struct {
	db       *database
	ptr      unsafe.Pointer // sqlite3_stmt* handle
	numInput int
}

// Exec executes the prepared statement with the given arguments.
func (s *statement) Exec(args ...any) (portin.Result, error) {
	if err := s.bind(args); err != nil {
		return nil, err
	}
	rc := s.db.capi.Sqlite3Step(s.ptr)
	if rc != sqliteDone && rc != sqliteRow {
		return nil, s.db.lastError(rc)
	}
	lastID := s.db.capi.Sqlite3LastInsertRowid(s.db.ptr)
	changes := s.db.capi.Sqlite3Changes(s.db.ptr)
	s.db.capi.Sqlite3Reset(s.ptr)
	return &result{lastID: lastID, changes: int64(changes)}, nil
}

// Query executes the prepared statement and returns rows.
func (s *statement) Query(args ...any) (portin.Rows, error) {
	if err := s.bind(args); err != nil {
		return nil, err
	}
	return &rows{
		stmt:      s,
		ownsStmt:  false,
		exhausted: false,
	}, nil
}

// NumInput returns the number of bind parameters.
func (s *statement) NumInput() int {
	return s.numInput
}

// Close finalizes the prepared statement.
func (s *statement) Close() error {
	if s.ptr == nil {
		return nil
	}
	rc := s.db.capi.Sqlite3Finalize(s.ptr)
	s.ptr = nil
	if rc != sqliteOK {
		return s.db.lastError(rc)
	}
	return nil
}

// bind binds arguments to the prepared statement. SQLite uses 1-based indices.
func (s *statement) bind(args []any) error {
	if len(args) == 0 {
		return nil
	}
	s.db.capi.Sqlite3Reset(s.ptr)
	s.db.capi.Sqlite3ClearBindings(s.ptr)

	for i, arg := range args {
		idx := uintptr(i + 1) // SQLite bind indices are 1-based
		var rc int32
		switch v := arg.(type) {
		case nil:
			rc = s.db.capi.Sqlite3BindNull(s.ptr, idx)
		case int:
			rc = s.db.capi.Sqlite3BindInt64(s.ptr, idx, uintptr(int64(v)))
		case int32:
			rc = s.db.capi.Sqlite3BindInt64(s.ptr, idx, uintptr(int64(v)))
		case int64:
			rc = s.db.capi.Sqlite3BindInt64(s.ptr, idx, uintptr(v))
		case float64:
			rc = s.db.capi.Sqlite3BindDouble(s.ptr, idx, v)
		case float32:
			rc = s.db.capi.Sqlite3BindDouble(s.ptr, idx, float64(v))
		case bool:
			var n int64
			if v {
				n = 1
			}
			rc = s.db.capi.Sqlite3BindInt64(s.ptr, idx, uintptr(n))
		case string:
			rc = s.db.capi.Sqlite3BindText(s.ptr, idx, v, uintptr(len(v)), sqliteTransient())
		case []byte:
			if len(v) == 0 {
				rc = s.db.capi.Sqlite3BindZeroblob(s.ptr, idx, 0)
			} else {
				if len(v) > math.MaxInt32 {
					return fmt.Errorf("blob too large (%d bytes)", len(v))
				}
				rc = s.db.capi.Sqlite3BindBlob(s.ptr, idx, unsafe.Pointer(&v[0]), int32(len(v)), sqliteTransient())
			}
		default:
			return fmt.Errorf("sqlite3: unsupported bind type %T at index %d", arg, i)
		}
		if rc != sqliteOK {
			return s.db.lastError(rc)
		}
	}
	return nil
}
