package driver

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/bnema/purego-sqlite/sqlite"
)

// sqliteTimeFormats are the datetime formats SQLite's date/time functions produce.
// Ordered from most to least specific so the first match wins.
var sqliteTimeFormats = []string{
	"2006-01-02 15:04:05.999999999-07:00",
	"2006-01-02T15:04:05.999999999-07:00",
	"2006-01-02 15:04:05.999999999Z",
	"2006-01-02T15:04:05.999999999Z",
	"2006-01-02 15:04:05.999999999",
	"2006-01-02T15:04:05.999999999",
	"2006-01-02 15:04:05Z",
	"2006-01-02T15:04:05Z",
	"2006-01-02 15:04:05",
	"2006-01-02T15:04:05",
	"2006-01-02 15:04",
	"2006-01-02T15:04",
	"2006-01-02",
}

// Driver implements database/sql/driver.Driver.
type Driver struct{}

// Open opens a new SQLite database connection.
func (d *Driver) Open(dsn string) (driver.Conn, error) {
	db, err := sqlite.Open(dsn)
	if err != nil {
		return nil, err
	}
	return &conn{db: db}, nil
}

// conn implements database/sql/driver.Conn and its context-aware extensions.
type conn struct {
	db sqlite.DB

	// opMu serializes SQLite calls on this connection. In particular, a query
	// retains it until its Rows are closed, so a late cancellation cannot
	// interrupt a later operation after this connection is returned to a pool.
	opMu     sync.Mutex
	activeMu sync.Mutex
	active   *operation
}

type operation struct {
	done chan struct{}
	once sync.Once
}

func (c *conn) beginOperation(ctx context.Context) (*operation, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	c.opMu.Lock()
	if err := ctx.Err(); err != nil {
		c.opMu.Unlock()
		return nil, err
	}
	// A context without a Done channel cannot cancel. It still holds opMu to
	// serialize the SQLite connection, but avoids allocating a token/watcher.
	if ctx.Done() == nil {
		return nil, nil
	}
	op := &operation{done: make(chan struct{})}
	c.activeMu.Lock()
	c.active = op
	c.activeMu.Unlock()
	go func() {
		select {
		case <-ctx.Done():
			c.activeMu.Lock()
			if c.active == op {
				c.db.Interrupt()
			}
			c.activeMu.Unlock()
		case <-op.done:
		}
	}()
	return op, nil
}

func (c *conn) finishOperation(op *operation) {
	if op == nil {
		c.opMu.Unlock()
		return
	}
	op.once.Do(func() {
		c.activeMu.Lock()
		if c.active == op {
			c.active = nil
		}
		close(op.done)
		c.activeMu.Unlock()
		c.opMu.Unlock()
	})
}

// Prepare returns a prepared statement.
func (c *conn) Prepare(query string) (driver.Stmt, error) {
	s, err := c.db.Prepare(query)
	if err != nil {
		return nil, err
	}
	return &stmt{s: s, conn: c}, nil
}

// PrepareContext prepares a statement without database/sql's legacy fallback.
func (c *conn) PrepareContext(ctx context.Context, query string) (driver.Stmt, error) {
	op, err := c.beginOperation(ctx)
	if err != nil {
		return nil, err
	}
	defer c.finishOperation(op)
	s, err := c.db.Prepare(query)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		return nil, err
	}
	return &stmt{s: s, conn: c}, nil
}

// Close closes the connection.
func (c *conn) Close() error {
	c.opMu.Lock()
	defer c.opMu.Unlock()
	return c.db.Close()
}

// Begin starts a transaction.
func (c *conn) Begin() (driver.Tx, error) { return c.begin() }

func (c *conn) begin() (driver.Tx, error) {
	if _, err := c.db.Exec("BEGIN"); err != nil {
		return nil, err
	}
	return &tx{db: c.db}, nil
}

// BeginTx starts a transaction directly while honoring context cancellation.
func (c *conn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	if opts.ReadOnly {
		return nil, errors.New("sqlite3: read-only transactions are not supported")
	}
	if opts.Isolation != driver.IsolationLevel(sql.LevelDefault) && opts.Isolation != driver.IsolationLevel(sql.LevelSerializable) {
		return nil, errors.New("sqlite3: requested isolation level is not supported")
	}
	op, err := c.beginOperation(ctx)
	if err != nil {
		return nil, err
	}
	defer c.finishOperation(op)
	if _, err := c.db.Exec("BEGIN"); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		return nil, err
	}
	return &tx{db: c.db}, nil
}

// ExecContext executes directly without database/sql's prepare/close fallback.
func (c *conn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	op, err := c.beginOperation(ctx)
	if err != nil {
		return nil, err
	}
	defer c.finishOperation(op)
	result, err := c.db.Exec(query, namedValuesToAny(args)...)
	if err != nil && ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return result, err
}

// QueryContext executes directly without database/sql's prepare/close fallback.
// Its operation remains owned by the returned Rows until Close or exhaustion.
func (c *conn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	op, err := c.beginOperation(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := c.db.Query(query, namedValuesToAny(args)...)
	if err != nil {
		c.finishOperation(op)
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, err
	}
	declTypes, err := rows.ColumnDeclTypes()
	if err != nil {
		closeErr := rows.Close()
		c.finishOperation(op)
		return nil, errors.Join(err, closeErr)
	}
	return &driverRows{rows: rows, declTypes: declTypes, ctx: ctx, release: func() { c.finishOperation(op) }}, nil
}

// tx implements database/sql/driver.Tx.
type tx struct{ db sqlite.DB }

func (t *tx) Commit() error   { _, err := t.db.Exec("COMMIT"); return err }
func (t *tx) Rollback() error { _, err := t.db.Exec("ROLLBACK"); return err }

// stmt implements database/sql/driver.Stmt and its context-aware extensions.
type stmt struct {
	s    sqlite.Stmt
	conn *conn
}

func (s *stmt) Close() error  { return s.s.Close() }
func (s *stmt) NumInput() int { return s.s.NumInput() }
func (s *stmt) Exec(args []driver.Value) (driver.Result, error) {
	return s.s.Exec(driverValuesToAny(args)...)
}
func (s *stmt) Query(args []driver.Value) (driver.Rows, error) {
	return s.query(driverValuesToAny(args), context.Background(), nil)
}

func (s *stmt) ExecContext(ctx context.Context, args []driver.NamedValue) (driver.Result, error) {
	if s.conn == nil {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return s.s.Exec(namedValuesToAny(args)...)
	}
	op, err := s.conn.beginOperation(ctx)
	if err != nil {
		return nil, err
	}
	defer s.conn.finishOperation(op)
	result, err := s.s.Exec(namedValuesToAny(args)...)
	if err != nil && ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return result, err
}

func (s *stmt) QueryContext(ctx context.Context, args []driver.NamedValue) (driver.Rows, error) {
	if s.conn == nil {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return s.query(namedValuesToAny(args), ctx, nil)
	}
	op, err := s.conn.beginOperation(ctx)
	if err != nil {
		return nil, err
	}
	return s.query(namedValuesToAny(args), ctx, func() { s.conn.finishOperation(op) })
}

func (s *stmt) query(args []any, ctx context.Context, release func()) (driver.Rows, error) {
	rows, err := s.s.Query(args...)
	if err != nil {
		if release != nil {
			release()
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, err
	}
	declTypes, err := rows.ColumnDeclTypes()
	if err != nil {
		closeErr := rows.Close()
		if release != nil {
			release()
		}
		return nil, errors.Join(err, closeErr)
	}
	return &driverRows{rows: rows, declTypes: declTypes, ctx: ctx, release: release}, nil
}

// driverRows implements database/sql/driver.Rows.
type driverRows struct {
	rows      sqlite.Rows
	declTypes []string
	values    []any
	ptrs      []any
	ctx       context.Context
	release   func()
	closeOnce sync.Once
	closeErr  error
}

// Columns returns the column names.
func (r *driverRows) Columns() []string { cols, _ := r.rows.Columns(); return cols }

// Close closes the rows and releases its cancellation ownership.
func (r *driverRows) Close() error {
	r.closeOnce.Do(func() {
		r.closeErr = r.rows.Close()
		if r.release != nil {
			r.release()
		}
	})
	return r.closeErr
}

// Next fills dest with the next row's values and releases ownership at EOF.
func (r *driverRows) Next(dest []driver.Value) error {
	if !r.rows.Next() {
		err := r.rows.Err()
		closeErr := r.Close()
		if r.ctx != nil && r.ctx.Err() != nil {
			return errors.Join(r.ctx.Err(), err, closeErr)
		}
		if err != nil || closeErr != nil {
			return errors.Join(err, closeErr)
		}
		return io.EOF
	}
	// driver.Value is a distinct named type, so Scan needs *any scratch slots.
	if len(r.values) != len(dest) {
		r.values = make([]any, len(dest))
		r.ptrs = make([]any, len(dest))
		for i := range dest {
			r.ptrs[i] = &r.values[i]
		}
	}
	if err := r.rows.Scan(r.ptrs...); err != nil {
		return errors.Join(err, r.Close())
	}
	for i := range dest {
		dest[i] = maybeParseDeclaredTime(r.declTypes, i, r.values[i])
	}
	return nil
}

// maybeParseDeclaredTime converts values for SQLite DATE, DATETIME, and TIMESTAMP columns.
func maybeParseDeclaredTime(declTypes []string, index int, v any) any {
	if index >= len(declTypes) || !isDateTimeDeclType(declTypes[index]) {
		return v
	}
	s, ok := v.(string)
	if !ok || len(s) < 10 {
		return v
	}
	for _, format := range sqliteTimeFormats {
		if t, err := time.ParseInLocation(format, s, time.UTC); err == nil {
			return t
		}
	}
	return v
}
func isDateTimeDeclType(declType string) bool {
	switch strings.ToUpper(strings.TrimSpace(declType)) {
	case "DATE", "DATETIME", "TIMESTAMP":
		return true
	}
	return false
}

// driverValuesToAny formats time.Time values as SQLite-compatible timestamps.
func driverValuesToAny(args []driver.Value) []any {
	out := make([]any, len(args))
	for i, v := range args {
		out[i] = normalizeValue(v)
	}
	return out
}
func namedValuesToAny(args []driver.NamedValue) []any {
	out := make([]any, len(args))
	for i := range args {
		out[i] = normalizeValue(args[i].Value)
	}
	return out
}
func normalizeValue(v any) any {
	if t, ok := v.(time.Time); ok {
		return t.UTC().Format("2006-01-02 15:04:05.999999999")
	}
	return v
}
