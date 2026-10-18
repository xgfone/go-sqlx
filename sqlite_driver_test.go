// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/xgfone/go-sqlx/dialect"
)

// A persistent SQLite process lets tests exercise database/sql transactions and
// the library's real execution path without adding a driver dependency.
func openSQLiteTestDB(t *testing.T) *DB {
	t.Helper()
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 unavailable")
	}

	std := sql.OpenDB(sqliteTestConnector{
		python: python,
		ctx:    t.Context(),
		path:   filepath.Join(t.TempDir(), "test.sqlite"),
	})
	std.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = std.Close() })
	return &DB{Dialect: dialect.SQLite, Executor: std}
}

func TestSQLiteTestDBReconnect(t *testing.T) {
	db := openSQLiteTestDB(t)
	sqliteInsertSchema(t, db, false)
	_, err := db.ExecContext(t.Context(), "INSERT INTO users(name) VALUES ('retained')")
	if err != nil {
		t.Fatal(err)
	}

	std := db.Executor.(*sql.DB)
	conn, err := std.Conn(t.Context())
	if err != nil {
		t.Fatal(err)
	}

	// Cancellation rollback can discard a physical connection in the same way.
	err = conn.Raw(func(any) error { return driver.ErrBadConn })
	if !errors.Is(err, driver.ErrBadConn) {
		t.Fatal(err)
	}

	_ = conn.Close()
	if names := sqliteInsertNames(t, db); names != "retained" {
		t.Fatal("reconnecting lost the SQLite test database", names)
	}
}

type sqliteTestConnector struct {
	python string
	ctx    context.Context
	path   string
}

func (c sqliteTestConnector) Driver() driver.Driver { return txDriver{} }
func (c sqliteTestConnector) Connect(ctx context.Context) (driver.Conn, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	script := `import json,sqlite3,sys
db=sqlite3.connect(sys.argv[1],isolation_level=None)
db.execute('PRAGMA foreign_keys=ON')
for line in sys.stdin:
    request=json.loads(line)
    try:
        cursor=db.execute(request['SQL'],request['Args'] or [])
        columns=[c[0] for c in cursor.description] if cursor.description else []
        rows=cursor.fetchall() if columns else []
        changed,last=db.execute('SELECT changes(),last_insert_rowid()').fetchone()
        result={'Columns':columns,'Rows':rows,'Changed':changed,'Last':last}
    except Exception as error:
        result={'Error':str(error)}
    print(json.dumps(result),flush=True)
`
	cmd := exec.CommandContext(c.ctx, c.python, "-u", "-c", script, c.path)
	cmd.Stderr = os.Stderr
	in, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}

	out, err := cmd.StdoutPipe()
	if err != nil {
		_ = in.Close()
		return nil, err
	}

	if err := cmd.Start(); err != nil {
		_ = in.Close()
		_ = out.Close()
		return nil, err
	}

	decoder := json.NewDecoder(out)
	decoder.UseNumber()
	return &sqliteTestConn{
		cmd: cmd,
		in:  in,

		encoder: json.NewEncoder(in),
		decoder: decoder,
	}, nil
}

type sqliteTestConn struct {
	cmd     *exec.Cmd
	in      io.WriteCloser
	encoder *json.Encoder
	decoder *json.Decoder
}

type sqliteTestResponse struct {
	Columns []string
	Rows    [][]any
	Changed int64
	Last    int64
	Error   string
}

func (c *sqliteTestConn) request(ctx context.Context, query string, args []driver.NamedValue) (sqliteTestResponse, error) {
	var response sqliteTestResponse
	if err := ctx.Err(); err != nil {
		return response, err
	}

	values := make([]any, len(args))
	for i, arg := range args {
		values[i] = arg.Value
	}

	if err := c.encoder.Encode(InsertStatement{SQL: query, Args: values}); err != nil {
		return response, err
	}
	if err := c.decoder.Decode(&response); err != nil {
		return response, err
	}
	if response.Error != "" {
		return response, errors.New(response.Error)
	}
	return response, nil
}

func (c *sqliteTestConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("SQLite test connection does not prepare statements")
}

func (c *sqliteTestConn) Close() error {
	_ = c.in.Close()
	return c.cmd.Wait()
}

func (c *sqliteTestConn) Begin() (driver.Tx, error) {
	_, err := c.request(context.Background(), "BEGIN", nil)
	return sqliteTestTx{c}, err
}

func (c *sqliteTestConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	if opts.Isolation != 0 || opts.ReadOnly {
		return nil, errors.New("unsupported SQLite test transaction options")
	}
	_, err := c.request(ctx, "BEGIN", nil)
	return sqliteTestTx{c}, err
}

func (c *sqliteTestConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	r, err := c.request(ctx, query, args)
	return insertTestResult{r.Last, r.Changed, nil}, err
}

func (c *sqliteTestConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	r, err := c.request(ctx, query, args)
	return &sqliteTestRows{columns: r.Columns, rows: r.Rows}, err
}

type sqliteTestTx struct{ conn *sqliteTestConn }

func (t sqliteTestTx) Commit() error {
	_, err := t.conn.request(context.Background(), "COMMIT", nil)
	if err != nil {
		// database/sql marks a transaction done even when Commit fails. SQLite
		// drivers must end a still-active transaction before reusing the connection.
		_, rollbackErr := t.conn.request(context.Background(), "ROLLBACK", nil)
		return errors.Join(err, rollbackErr)
	}
	return err
}

func (t sqliteTestTx) Rollback() error {
	_, err := t.conn.request(context.Background(), "ROLLBACK", nil)
	return err
}

type sqliteTestRows struct {
	columns []string
	rows    [][]any
}

func (r *sqliteTestRows) Columns() []string { return r.columns }
func (r *sqliteTestRows) Close() error      { return nil }
func (r *sqliteTestRows) Next(dest []driver.Value) error {
	if len(r.rows) == 0 {
		return io.EOF
	}

	for i, value := range r.rows[0] {
		if n, ok := value.(json.Number); ok {
			integer, err := n.Int64()
			if err != nil {
				return fmt.Errorf("invalid SQLite test integer: %w", err)
			}
			value = integer
		}
		dest[i] = value
	}
	r.rows = r.rows[1:]
	return nil
}
