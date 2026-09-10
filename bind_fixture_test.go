// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"sync/atomic"
	"testing"

	"github.com/xgfone/go-sqlx/dialect"
)

// A real database/sql driver with controllable iteration/finalization failures.
// Each cursor owns its reusable byte buffers; the fixture is safe to share.
type bindFixture struct {
	columns           []string
	values            [][]driver.Value
	nextErr, closeErr error
	next, closed      atomic.Int64
}

type bindConnector struct{ fixture *bindFixture }

func (c bindConnector) Connect(context.Context) (driver.Conn, error) {
	return &bindConn{c.fixture}, nil
}

func (bindConnector) Driver() driver.Driver {
	return fixtureDriver{}
}

type bindConn struct{ fixture *bindFixture }

func (*bindConn) Close() error                        { return nil }
func (*bindConn) Begin() (driver.Tx, error)           { return nil, errors.New("unexpected Begin") }
func (*bindConn) Prepare(string) (driver.Stmt, error) { return nil, errors.New("unexpected Prepare") }

func (c *bindConn) QueryContext(context.Context, string, []driver.NamedValue) (driver.Rows, error) {
	return &bindDriverRows{
		fixture: c.fixture,
		buffers: make([][]byte, len(c.fixture.columns)),
	}, nil
}

type bindDriverRows struct {
	fixture *bindFixture
	index   int
	buffers [][]byte
}

func (r *bindDriverRows) Columns() []string { return r.fixture.columns }

func (r *bindDriverRows) Close() error {
	r.fixture.closed.Add(1)
	// Closing invalidates borrowed driver memory as well as Next.
	for _, buffer := range r.buffers {
		for i := range buffer {
			buffer[i] = '!'
		}
	}
	return r.fixture.closeErr
}

func (r *bindDriverRows) Next(dst []driver.Value) error {
	r.fixture.next.Add(1)
	if r.index >= len(r.fixture.values) {
		if r.fixture.nextErr != nil {
			return r.fixture.nextErr
		}
		return io.EOF
	}

	for i, value := range r.fixture.values[r.index] {
		if data, ok := value.([]byte); ok {
			r.buffers[i] = append(r.buffers[i][:0], data...)
			value = r.buffers[i]
		}
		dst[i] = value
	}

	r.index++
	return nil
}

func bindTestDB(t testing.TB, f *bindFixture) *DB {
	t.Helper()
	std := sql.OpenDB(bindConnector{f})
	t.Cleanup(func() { _ = std.Close() })
	return &DB{Dialect: dialect.SQLite, Executor: std}
}

func bindTestRows(t testing.TB, values ...driver.Value) (Rows, *bindFixture) {
	t.Helper()
	f := &bindFixture{columns: []string{"value"}}
	for _, value := range values {
		f.values = append(f.values, []driver.Value{value})
	}
	return bindTestDB(t, f).QueryRowsContext(context.Background(), "SELECT value"), f
}
