// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"strings"
	"testing"

	"github.com/xgfone/go-sqlx/dialect"
)

// These fixture transactions exercise database/sql's actual transaction dispatch.
type txConnector struct{ f *txFixture }

type txDriver struct{}

type txFixture struct {
	begun     int
	execs     int
	rolled    int
	committed int
	query     string
}

func (txDriver) Open(string) (driver.Conn, error) { return nil, errors.New("connector required") }

func (c txConnector) Driver() driver.Driver { return txDriver{} }

func (c txConnector) Connect(context.Context) (driver.Conn, error) { return &txConn{c.f}, nil }

type txConn struct{ f *txFixture }

func (c *txConn) Prepare(string) (driver.Stmt, error) { return nil, errors.New("unexpected prepare") }

func (c *txConn) Close() error { return nil }

func (c *txConn) Begin() (driver.Tx, error) { c.f.begun++; return &txState{c.f}, nil }

func (c *txConn) ExecContext(_ context.Context, q string, _ []driver.NamedValue) (driver.Result, error) {
	c.f.execs++
	c.f.query = q
	return driver.RowsAffected(1), nil
}

type txState struct{ f *txFixture }

func (t *txState) Commit() error { t.f.committed++; return nil }

func (t *txState) Rollback() error { t.f.rolled++; return nil }

func TestTransactionExecutor(t *testing.T) {
	f := &txFixture{}
	std := sql.OpenDB(txConnector{f})
	defer std.Close() //nolint:errcheck

	db := &DB{Dialect: dialect.Postgres, Executor: std}
	tx, e := db.BeginTx(context.Background(), nil)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback() //nolint:errcheck

	_, e = db.Update().Table("t").Set(Set("value", 7)).SetExecutor(tx).ExecContext(context.Background())
	if e != nil {
		t.Fatal(e)
	}

	_, e = db.WithExecutor(tx).Delete().From("t").Where(Eq("id", 1)).ExecContext(context.Background())
	if e != nil {
		t.Fatal(e)
	}

	if e = tx.Commit(); e != nil {
		t.Fatal(e)
	}

	if f.begun != 1 || f.committed != 1 || f.execs != 2 {
		t.Fatalf("%+v", f)
	}

	_, e = db.WithExecutor(tx).Insert().Into("t").Values(1).ExecContext(context.Background())
	if !errors.Is(e, sql.ErrTxDone) {
		t.Fatal(e)
	}
}

type nilRowsExecutor struct{ Executor }

func (nilRowsExecutor) QueryContext(context.Context, string, ...any) (*sql.Rows, error) {
	return nil, nil
}

func TestBuilderQueryRejectsNilExecutorRows(t *testing.T) {
	db := &DB{Dialect: dialect.Postgres, Executor: nilRowsExecutor{}}
	for _, err := range []error{
		db.Select("id").QueryRowsContext(context.Background()).Err(),
		db.Select("id").QueryRowContext(context.Background()).Err(),
		db.Insert().Into("t").Values(1).Returning("id").QueryRowsContext(context.Background()).Err(),
	} {
		if err == nil || !strings.Contains(err.Error(), "nil rows") {
			t.Fatalf("expected nil rows error, got %v", err)
		}
	}
}
