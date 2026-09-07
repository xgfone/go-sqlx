// Copyright 2026 xgfone
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package sqlx

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"reflect"
	"testing"

	"github.com/xgfone/go-op"
	"github.com/xgfone/go-sqlx/dialect"
)

type scanFixture struct {
	values   []driver.Value
	received []driver.NamedValue
	query    string
}
type fixtureConnector struct{ fixture *scanFixture }

func (c fixtureConnector) Connect(context.Context) (driver.Conn, error) {
	return &fixtureConn{fixture: c.fixture}, nil
}
func (c fixtureConnector) Driver() driver.Driver { return fixtureDriver{} }

type fixtureDriver struct{}

func (fixtureDriver) Open(string) (driver.Conn, error) { return nil, errors.New("use connector") }

type fixtureConn struct{ fixture *scanFixture }

func (*fixtureConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("unexpected Prepare")
}
func (*fixtureConn) Close() error              { return nil }
func (*fixtureConn) Begin() (driver.Tx, error) { return nil, errors.New("unexpected Begin") }
func (c *fixtureConn) QueryContext(_ context.Context, q string, args []driver.NamedValue) (driver.Rows, error) {
	c.fixture.received = append([]driver.NamedValue(nil), args...)
	c.fixture.query = q
	return &fixtureRows{values: c.fixture.values}, nil
}

type fixtureRows struct {
	values []driver.Value
	index  int
	buffer []byte
}

func (*fixtureRows) Columns() []string { return []string{"value"} }
func (*fixtureRows) Close() error      { return nil }
func (r *fixtureRows) Next(dest []driver.Value) error {
	if r.index == len(r.values) {
		return io.EOF
	}

	source := r.values[r.index]
	r.index++

	if bytes, ok := source.([]byte); ok {
		r.buffer = append(r.buffer[:0], bytes...)
		source = r.buffer
	}

	dest[0] = source
	return nil
}

func fixtureDB(t *testing.T, f *scanFixture) *DB {
	t.Helper()
	std := sql.OpenDB(fixtureConnector{f})
	t.Cleanup(func() { _ = std.Close() })
	return &DB{Dialect: dialect.SQLite, Database: std}
}

func TestQueryContextNamedArgsAndBufferLifetime(t *testing.T) {
	fixture := &scanFixture{values: []driver.Value{[]byte("abc"), []byte("xyz")}}
	db := fixtureDB(t, fixture)
	rows := db.Select("value").From("t").Where(op.Eq("id", sql.Named("id", 7))).QueryRowsContext(context.Background())
	if rows.Err() != nil {
		t.Fatal(rows.Err())
	}
	defer rows.Close() //nolint:errcheck

	if fixture.query != `SELECT "value" FROM "t" WHERE "id"=@id` ||
		len(fixture.received) != 1 || fixture.received[0].Name != "id" ||
		fixture.received[0].Value != int64(7) {
		t.Fatalf("%q %#v", fixture.query, fixture.received)
	}

	var first, second any
	if !rows.Next() {
		t.Fatal("missing first row")
	}
	if err := rows.Scan(&first); err != nil {
		t.Fatal(err)
	}
	if !rows.Next() {
		t.Fatal("missing second row")
	}
	if err := rows.Scan(&second); err != nil {
		t.Fatal(err)
	}
	if string(first.([]byte)) != "abc" || string(second.([]byte)) != "xyz" {
		t.Fatal(first, second)
	}
}

func TestQueryScannerResetsReusedStruct(t *testing.T) {
	fixture := &scanFixture{values: []driver.Value{int64(7), nil}}
	rows := fixtureDB(t, fixture).Select("value").From("t").QueryRows()
	defer rows.Close() //nolint:errcheck

	type scalar int64
	var got []scalar
	var record struct {
		Value scalar `sql:"value"`
	}

	for rows.Next() {
		if err := rows.Scan(&record); err != nil {
			t.Fatal(err)
		}
		got = append(got, record.Value)
	}

	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, []scalar{7, 0}) {
		t.Fatal(got)
	}
}

func TestQueryRowPreservesBuilderLimit(t *testing.T) {
	fixture := &scanFixture{}
	db := fixtureDB(t, fixture)
	builder := db.Select("value").From("t").Limit(0)

	var value int
	if err := builder.QueryRow().Scan(&value); err != sql.ErrNoRows {
		t.Fatal(err)
	}
	if fixture.query != `SELECT "value" FROM "t" LIMIT 0` {
		t.Fatal(fixture.query)
	}

	unlimited := db.Select("value").From("t")
	if err := unlimited.QueryRow().Scan(&value); err != sql.ErrNoRows {
		t.Fatal(err)
	}
	if fixture.query != `SELECT "value" FROM "t" LIMIT 1` {
		t.Fatal(fixture.query)
	}
	if q, _ := unlimited.Build(); q != `SELECT "value" FROM "t"` {
		t.Fatal("QueryRow modified builder:", q)
	}
}
