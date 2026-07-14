package core

import (
	"fmt"
	"unsafe"

	portin "github.com/bnema/purego-sqlite/internal/ports/in"
)

// Compile-time interface checks.
var _ portin.Rows = (*rows)(nil)
var _ portin.Result = (*result)(nil)

// rows implements portin.Rows.
type rows struct {
	stmt      *statement
	ownsStmt  bool // true when rows owns the stmt lifecycle (from db.Query)
	exhausted bool
	hasRow    bool // true after a successful Step returning SQLITE_ROW
	lastErr   error
}

// Next advances to the next row. Returns false when exhausted or on error.
func (r *rows) Next() bool {
	if r.exhausted {
		return false
	}
	rc := r.stmt.db.capi.Sqlite3Step(r.stmt.ptr)
	switch rc {
	case sqliteRow:
		r.hasRow = true
		return true
	case sqliteDone:
		r.exhausted = true
		r.hasRow = false
		return false
	default:
		r.lastErr = r.stmt.db.lastError(rc)
		r.exhausted = true
		r.hasRow = false
		return false
	}
}

// Scan reads the current row's columns into dest pointers.
// Supported dest types: *int, *int64, *float64, *string, *[]byte, *bool, *any.
func (r *rows) Scan(dest ...any) error {
	if r.lastErr != nil {
		return r.lastErr
	}
	if !r.hasRow {
		return fmt.Errorf("sqlite3: Scan called without a valid row")
	}

	capi := r.stmt.db.capi
	stmtPtr := r.stmt.ptr
	ncols := int(capi.Sqlite3ColumnCount(stmtPtr))
	if len(dest) > ncols {
		return fmt.Errorf("sqlite3: Scan expected at most %d dest arguments, got %d", ncols, len(dest))
	}

	for i, d := range dest {
		col := int32(i)
		colType := capi.Sqlite3ColumnType(stmtPtr, col)

		switch dp := d.(type) {
		case *int:
			*dp = int(capi.Sqlite3ColumnInt64(stmtPtr, col))
		case *int64:
			*dp = capi.Sqlite3ColumnInt64(stmtPtr, col)
		case *float64:
			*dp = capi.Sqlite3ColumnDouble(stmtPtr, col)
		case *string:
			*dp = columnText(capi, stmtPtr, col)
		case *[]byte:
			if colType == sqliteNull {
				*dp = nil
			} else {
				blobPtr := capi.Sqlite3ColumnBlob(stmtPtr, col)
				nbytes := capi.Sqlite3ColumnBytes(stmtPtr, col)
				if nbytes == 0 || blobPtr == nil {
					*dp = nil
				} else {
					buf := make([]byte, nbytes)
					copy(buf, unsafe.Slice((*byte)(blobPtr), nbytes))
					*dp = buf
				}
			}
		case *bool:
			*dp = capi.Sqlite3ColumnInt64(stmtPtr, col) != 0
		case *any:
			switch colType {
			case sqliteInteger:
				*dp = capi.Sqlite3ColumnInt64(stmtPtr, col)
			case sqliteFloat:
				*dp = capi.Sqlite3ColumnDouble(stmtPtr, col)
			case sqliteText:
				*dp = columnText(capi, stmtPtr, col)
			case sqliteBlob:
				blobPtr := capi.Sqlite3ColumnBlob(stmtPtr, col)
				nbytes := capi.Sqlite3ColumnBytes(stmtPtr, col)
				if nbytes == 0 || blobPtr == nil {
					*dp = []byte(nil)
				} else {
					buf := make([]byte, nbytes)
					copy(buf, unsafe.Slice((*byte)(blobPtr), nbytes))
					*dp = buf
				}
			case sqliteNull:
				*dp = nil
			}
		default:
			return fmt.Errorf("sqlite3: unsupported Scan dest type %T at index %d", d, i)
		}
	}
	return nil
}

// Err returns the error, if any, that was encountered during iteration.
func (r *rows) Err() error {
	return r.lastErr
}

// Columns returns the column names for the result set.
func (r *rows) Columns() ([]string, error) {
	capi := r.stmt.db.capi
	stmtPtr := r.stmt.ptr
	ncols := int(capi.Sqlite3ColumnCount(stmtPtr))
	names := make([]string, ncols)
	for i := 0; i < ncols; i++ {
		names[i] = capi.Sqlite3ColumnName(stmtPtr, int32(i))
	}
	return names, nil
}

// Close resets the statement (and finalizes it if rows owns it).
func (r *rows) Close() error {
	if r.stmt == nil {
		return nil
	}
	if r.ownsStmt {
		err := r.stmt.Close()
		r.stmt = nil
		return err
	}
	r.stmt.db.capi.Sqlite3Reset(r.stmt.ptr)
	r.stmt = nil
	return nil
}

// columnText extracts a string from Sqlite3ColumnText (*byte) return.
func columnText(capi interface {
	Sqlite3ColumnText(unsafe.Pointer, int32) *byte
	Sqlite3ColumnBytes(unsafe.Pointer, int32) int32
}, stmtPtr unsafe.Pointer, col int32) string {
	textPtr := capi.Sqlite3ColumnText(stmtPtr, col)
	if textPtr == nil {
		return ""
	}
	nbytes := capi.Sqlite3ColumnBytes(stmtPtr, col)
	if nbytes == 0 {
		return ""
	}
	return string(unsafe.Slice(textPtr, nbytes))
}

// result implements portin.Result.
type result struct {
	lastID  int64
	changes int64
}

// LastInsertId returns the last inserted row ID.
func (r *result) LastInsertId() (int64, error) {
	return r.lastID, nil
}

// RowsAffected returns the number of rows affected by the statement.
func (r *result) RowsAffected() (int64, error) {
	return r.changes, nil
}
