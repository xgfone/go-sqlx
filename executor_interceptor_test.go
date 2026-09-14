// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"reflect"
	"testing"

	"github.com/xgfone/go-sqlx/dialect"
)

type executorLayer struct{ Executor }

func (e executorLayer) Unwrap() Executor { return e.Executor }

func TestAsExecutor(t *testing.T) {
	std := new(sql.DB)
	inner := &executorLayer{std}
	outer := &executorLayer{inner}

	// This layer is not comparable. Traversal must not require interface equality.
	e := struct {
		executorLayer
		values []int
	}{executorLayer{outer}, []int{1}}
	db := &DB{Executor: e}

	if got, ok := AsExecutor[*sql.DB](db); !ok || got != std {
		t.Fatal("underlying database not found", got, ok)
	}
	if got, ok := AsExecutor[*executorLayer](db); !ok || got != outer {
		t.Fatal("did not select the outermost match", got, ok)
	}
	if got, ok := AsExecutor[TxBeginner](e); !ok || got != std {
		t.Fatal("transaction capability not found", got, ok)
	}

	for _, e := range []Executor{
		nil,
		(*DB)(nil),
		executorLayer{},
		nilRowsExecutor{},
		db,
	} {
		if got, ok := AsExecutor[*sql.Tx](e); ok || got != nil {
			t.Fatal("unexpected match", got, ok)
		}
	}

	var nilTx *sql.Tx
	if got, ok := AsExecutor[*sql.Tx](executorLayer{nilTx}); !ok || got != nil {
		t.Fatal("typed nil did not match", got, ok)
	}
}

type executorCapabilities struct {
	executorLayer
	begin func(context.Context, *sql.TxOptions) (*sql.Tx, error)
	close func() error
}

func (e executorCapabilities) BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error) {
	return e.begin(ctx, opts)
}

func (e executorCapabilities) Close() error { return e.close() }

func TestDBWrappedCapabilitiesPreferOuter(t *testing.T) {
	f := &txFixture{}
	std := sql.OpenDB(txConnector{f})
	t.Cleanup(func() { _ = std.Close() })

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	opts := &sql.TxOptions{ReadOnly: true}
	failure := errors.New("outer capability failure")
	begun, closed := 0, 0
	db := &DB{Executor: executorLayer{executorCapabilities{
		executorLayer: executorLayer{std},
		begin: func(c context.Context, o *sql.TxOptions) (*sql.Tx, error) {
			begun++
			if c != ctx || o != opts {
				t.Fatal("transaction arguments changed")
			}
			return nil, failure
		},
		close: func() error { closed++; return failure },
	}}}

	if _, err := db.BeginTx(ctx, opts); !errors.Is(err, failure) {
		t.Fatal("outer transaction error lost", err)
	}
	if err := db.Close(); !errors.Is(err, failure) {
		t.Fatal("outer close error lost", err)
	}
	if begun != 1 || closed != 1 || f.begun != 0 {
		t.Fatal("outer capability was bypassed", begun, closed, f)
	}
	if err := std.PingContext(ctx); err != nil {
		t.Fatal("outer close failure closed the inner database", err)
	}
}

type interceptedCall struct {
	method string
	ctx    context.Context
	query  string
	args   []any
}

type interceptedExecutor struct {
	executorLayer
	calls *[]interceptedCall
}

func (e *interceptedExecutor) record(method string, ctx context.Context, query string, args []any) {
	*e.calls = append(*e.calls, interceptedCall{method, ctx, query, append([]any(nil), args...)})
}

func (e *interceptedExecutor) PrepareContext(ctx context.Context, query string) (*sql.Stmt, error) {
	e.record("prepare", ctx, query, nil)
	return e.Executor.PrepareContext(ctx, query)
}

func (e *interceptedExecutor) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	e.record("exec", ctx, query, args)
	return e.Executor.ExecContext(ctx, query, args...)
}

func (e *interceptedExecutor) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	e.record("query", ctx, query, args)
	return e.Executor.QueryContext(ctx, query, args...)
}

func (e *interceptedExecutor) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	e.record("queryrow", ctx, query, args)
	return e.Executor.QueryRowContext(ctx, query, args...)
}

func installTestInterceptor(t *testing.T) *[]interceptedCall {
	t.Helper()

	previous := defaultExecutorInterceptor
	t.Cleanup(func() { SetDefaultExecutorInterceptor(previous) })

	var calls []interceptedCall
	SetDefaultExecutorInterceptor(func(e Executor) Executor {
		return &interceptedExecutor{executorLayer{e}, &calls}
	})
	return &calls
}

func TestExecutorInterceptorTransactionBindings(t *testing.T) {
	calls := installTestInterceptor(t)
	f := &txFixture{}
	std := sql.OpenDB(txConnector{f})
	t.Cleanup(func() { _ = std.Close() })

	db := (&DB{Dialect: dialect.Postgres}).SetExecutor(executorLayer{std})
	ctx := context.Background()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback() //nolint:errcheck

	txdb := db.WithExecutor(tx)
	if got, ok := AsExecutor[*sql.Tx](txdb); !ok || got != tx {
		t.Fatal("lost transaction")
	}
	if _, ok := AsExecutor[*sql.DB](txdb); ok {
		t.Fatal("transaction resolved to the connection pool")
	}
	if _, err := txdb.BeginTx(ctx, nil); err == nil {
		t.Fatal("transaction unexpectedly supports nested transactions")
	}
	if err := txdb.Close(); err == nil {
		t.Fatal("transaction unexpectedly supports Close")
	}

	for _, b := range []interface {
		ExecContext(context.Context) (sql.Result, error)
	}{
		db.Insert().Into("t").Columns("value").Values(1).SetExecutor(tx),
		db.Update().Table("t").Set(Set("value", 2)).SetExecutor(tx),
		db.Delete().From("t").Where(Eq("value", 3)).SetExecutor(tx),
		txdb.Insert().Into("t").Columns("value").Values(4),
	} {
		if _, err := b.ExecContext(ctx); err != nil {
			t.Fatal(err)
		}
	}

	q := mustCompileTemplate(t, db.Update().Table("t").Set(Set("value", Param(0))))
	if _, err := q.ExecContext(ctx, txdb, 5); err != nil {
		t.Fatal(err)
	}
	if len(*calls) != 5 || f.execs != 5 {
		t.Fatal("missing or duplicate interception", len(*calls), f.execs)
	}

	for i, call := range *calls {
		if call.method != "exec" || call.ctx != ctx || call.query == "" ||
			!reflect.DeepEqual(call.args, []any{i + 1}) {
			t.Fatalf("call %d: %+v", i, call)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if f.begun != 1 || f.committed != 1 {
		t.Fatalf("unexpected transaction state: %+v", f)
	}

	_, err = txdb.ExecContext(ctx, "UPDATE t SET value = 6")
	if !errors.Is(err, sql.ErrTxDone) {
		t.Fatal("transaction error was lost", err)
	}
}

func TestExecutorInterceptorQueryAndPrepare(t *testing.T) {
	calls := installTestInterceptor(t)
	db := bindTestDB(t, &bindFixture{
		columns: []string{"value"},
		values:  [][]driver.Value{{int64(7)}},
	})
	std := db.Executor
	db.SetExecutor(std)

	ctx := context.Background()
	_, err := db.PrepareContext(ctx, "SELECT ?")
	if err == nil || err.Error() != "unexpected Prepare" {
		t.Fatal("prepare error was lost", err)
	}

	rows, err := db.QueryContext(ctx, "SELECT ?", 7)
	if err != nil {
		t.Fatal(err)
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}

	var value int
	err = db.QueryRowContext(ctx, "SELECT ?", 7).Scan(&value)
	if err != nil || value != 7 {
		t.Fatal(value, err)
	}
	err = db.Select("value").SetExecutor(std).QueryRowContext(ctx).Scan(&value)
	if err != nil || value != 7 {
		t.Fatal(value, err)
	}

	var values []int
	err = mustCompileTemplate(t, db.Select("value")).QueryRowsContext(ctx, db).Bind(&values)
	if err != nil || !reflect.DeepEqual(values, []int{7}) {
		t.Fatal(values, err)
	}

	var methods []string
	for _, call := range *calls {
		methods = append(methods, call.method)
	}
	if !reflect.DeepEqual(methods, []string{"prepare", "query", "queryrow", "query", "query"}) {
		t.Fatal("missing or duplicate interception", methods)
	}
}

func TestExecutorInterceptorOpen(t *testing.T) {
	calls := installTestInterceptor(t)
	previous := DefaultOpener
	t.Cleanup(func() { DefaultOpener = previous })

	std := sql.OpenDB(txConnector{&txFixture{}})
	t.Cleanup(func() { _ = std.Close() })

	DefaultOpener = func(name, dsn string) (*sql.DB, error) {
		if name != "postgres" || dsn != "test" {
			t.Fatal(name, dsn)
		}
		return std, nil
	}

	db, err := Open("postgres", "test", MaxOpenConns(3))
	if err != nil {
		t.Fatal(err)
	}

	got, ok := AsExecutor[*sql.DB](db)
	if !ok || got != std || got.Stats().MaxOpenConnections != 3 {
		t.Fatal("Open lost its underlying database or configuration")
	}

	_, err = db.ExecContext(context.Background(), "UPDATE t SET value = 1")
	if err != nil {
		t.Fatal(err)
	}
	if len(*calls) != 1 {
		t.Fatal("Open did not apply interceptor", len(*calls))
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := std.PingContext(context.Background()); err == nil {
		t.Fatal("underlying database was not closed")
	}
}

func TestExecutorInterceptorDefaultsAndNil(t *testing.T) {
	e := new(sql.DB)
	if new(DB).SetExecutor(e).Executor != e {
		t.Fatal("default interceptor must leave executor unchanged")
	}

	previous := defaultExecutorInterceptor
	t.Cleanup(func() { SetDefaultExecutorInterceptor(previous) })

	SetDefaultExecutorInterceptor(func(Executor) Executor {
		t.Fatal("nil executor passed to interceptor")
		return nil
	})

	db := (&DB{Executor: e}).SetExecutor(nil)
	if db.Executor != nil || db.WithExecutor(nil).Executor != nil {
		t.Fatal("nil executor was not preserved")
	}

	for _, b := range []*builderBase{
		&db.Select().SetExecutor(nil).builderBase,
		&db.Insert().SetExecutor(nil).builderBase,
		&db.Update().SetExecutor(nil).builderBase,
		&db.Delete().SetExecutor(nil).builderBase,
	} {
		if _, err := b.runner(); err == nil {
			t.Fatal("missing executor was accepted")
		}
	}

	SetDefaultExecutorInterceptor(nil)
	if db.SetExecutor(e).Executor != e {
		t.Fatal("reset interceptor must leave executor unchanged")
	}
}
