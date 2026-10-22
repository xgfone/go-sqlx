// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/xgfone/go-sqlx/dialect"
)

func TestDBTransactionLifecycle(t *testing.T) {
	callbackErr := errors.New("callback failed")
	commitErr := errors.New("commit failed")
	rollbackErr := errors.New("rollback failed")
	for _, tc := range []struct {
		name        string
		callbackErr error
		commitErr   error
		rollbackErr error
		wantErr     error
		wantRBErr   error
		committed   int
		rolled      int
	}{
		{name: "success", committed: 1},
		{name: "callback error", callbackErr: callbackErr, wantErr: callbackErr, rolled: 1},
		{name: "rollback error", callbackErr: callbackErr, rollbackErr: rollbackErr, wantErr: callbackErr, wantRBErr: rollbackErr, rolled: 1},
		{name: "rollback already done", callbackErr: callbackErr, rollbackErr: sql.ErrTxDone, wantErr: callbackErr, rolled: 1},
		{name: "wrapped rollback already done", callbackErr: callbackErr, rollbackErr: errors.Join(sql.ErrTxDone), wantErr: callbackErr, rolled: 1},
		{name: "commit error", commitErr: commitErr, wantErr: commitErr, committed: 1},
		// database/sql marks a transaction done before calling the driver's Commit.
		{name: "commit error marks transaction done", commitErr: commitErr, rollbackErr: rollbackErr, wantErr: commitErr, committed: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &txFixture{commitErr: tc.commitErr, rollbackErr: tc.rollbackErr}
			std := sql.OpenDB(txConnector{f})
			t.Cleanup(func() { _ = std.Close() })
			db := &DB{Dialect: dialect.Postgres, Executor: std}
			ctx := t.Context()

			var txdb *DB
			called := 0
			err := db.Transaction(ctx, nil, func(bound *DB) error {
				called++
				txdb = bound
				if txdb == db {
					t.Fatal("callback received the original DB")
				}

				_, err := txdb.Update().Table("t").Set(Set("value", 7)).ExecContext(ctx)
				if err != nil {
					return err
				}
				return tc.callbackErr
			})
			if tc.wantRBErr != nil {
				if !errors.Is(err, tc.wantErr) || !errors.Is(err, tc.wantRBErr) ||
					!strings.Contains(err.Error(), "sqlx: rollback transaction: ") {
					t.Fatalf("got error %v; want callback %v and rollback %v with context", err, tc.wantErr, tc.wantRBErr)
				}
			} else if err != tc.wantErr {
				t.Fatalf("got error %v; want unchanged %v", err, tc.wantErr)
			}
			if called != 1 || f.begun != 1 || f.execs != 1 ||
				f.committed != tc.committed || f.rolled != tc.rolled {
				t.Fatalf("callback calls %d, transaction state %+v", called, f)
			}
			if db.Executor != std {
				t.Fatal("original DB's executor changed")
			}

			_, err = txdb.ExecContext(ctx, "UPDATE t SET value = 8")
			if !errors.Is(err, sql.ErrTxDone) {
				t.Fatal("transaction remained usable after returning", err)
			}
		})
	}
}

func TestDBTransactionBeginFailure(t *testing.T) {
	beginErr := errors.New("begin failed")
	for _, tc := range []struct {
		name        string
		beginErr    error
		unsupported bool
		canceled    bool
		wantErr     error
		begun       int
	}{
		{name: "driver error", beginErr: beginErr, wantErr: beginErr, begun: 1},
		{name: "unsupported executor", unsupported: true, wantErr: ErrTxUnsupported},
		{name: "canceled context", canceled: true, wantErr: context.Canceled},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &txFixture{beginErr: tc.beginErr}
			std := sql.OpenDB(txConnector{f})
			t.Cleanup(func() { _ = std.Close() })

			db := &DB{Executor: std}
			if tc.unsupported {
				db.Executor = executorLayer{}
			}

			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if tc.canceled {
				cancel()
			}

			called := false
			err := db.Transaction(ctx, nil, func(*DB) error {
				called = true
				return nil
			})
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("got error %v; want %v", err, tc.wantErr)
			}
			if called || f.begun != tc.begun || f.committed != 0 || f.rolled != 0 {
				t.Fatalf("callback called %v, transaction state %+v", called, f)
			}
		})
	}
}

func TestDBTransactionArguments(t *testing.T) {
	for _, opts := range []*sql.TxOptions{
		nil,
		{Isolation: sql.LevelSerializable, ReadOnly: true},
	} {
		name := "default options"
		if opts != nil {
			name = "explicit options"
		}
		t.Run(name, func(t *testing.T) {
			f := &txFixture{}
			std := sql.OpenDB(txConnector{f})
			t.Cleanup(func() { _ = std.Close() })
			ctx := t.Context()
			begun := 0
			db := &DB{Executor: executorLayer{executorCapabilities{
				executorLayer: executorLayer{std},
				begin: func(c context.Context, o *sql.TxOptions) (*sql.Tx, error) {
					begun++
					if c != ctx || o != opts {
						t.Fatal("transaction arguments changed")
					}
					return std.BeginTx(c, o)
				},
			}}}

			err := db.Transaction(ctx, opts, func(*DB) error { return nil })
			if err != nil {
				t.Fatal(err)
			}

			var want driver.TxOptions
			if opts != nil {
				want = driver.TxOptions{
					Isolation: driver.IsolationLevel(opts.Isolation),
					ReadOnly:  opts.ReadOnly,
				}
			}
			if begun != 1 || f.begun != 1 || f.committed != 1 ||
				f.beginContext != ctx || f.options != want {
				t.Fatalf("outer begin calls %d, transaction state %+v; want options %+v", begun, f, want)
			}
		})
	}
}

func TestDBTransactionPanic(t *testing.T) {
	for _, tc := range []struct {
		name        string
		interceptor bool
		rollbackErr error
	}{
		{name: "callback"},
		{name: "interceptor", interceptor: true},
		{name: "callback with rollback error", rollbackErr: errors.New("rollback failed")},
		{name: "interceptor with rollback error", interceptor: true, rollbackErr: errors.New("rollback failed")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &txFixture{rollbackErr: tc.rollbackErr}
			std := sql.OpenDB(txConnector{f})
			t.Cleanup(func() { _ = std.Close() })
			db := &DB{Executor: std}
			panicValue := new(int)
			wantCalls := 1
			if tc.interceptor {
				previous := defaultExecutorInterceptor
				t.Cleanup(func() { SetDefaultExecutorInterceptor(previous) })
				SetDefaultExecutorInterceptor(func(Executor) Executor { panic(panicValue) })
				wantCalls = 0
			}

			var recovered any
			called := 0
			func() {
				defer func() { recovered = recover() }()
				_ = db.Transaction(t.Context(), nil, func(*DB) error {
					called++
					panic(panicValue)
				})
			}()
			if recovered != panicValue || called != wantCalls ||
				f.begun != 1 || f.committed != 0 || f.rolled != 1 {
				t.Fatalf("panic %v, callback calls %d, transaction state %+v", recovered, called, f)
			}
		})
	}
}

func TestDBTransactionConfigurationAndInterceptor(t *testing.T) {
	calls := installTestInterceptor(t)
	f := &txFixture{}
	std := sql.OpenDB(txConnector{f})
	t.Cleanup(func() { _ = std.Close() })
	config := BindConfig{
		Capacity:      73,
		DuplicateKeys: DuplicateKeyLast,
		ScanOptions: ScanOptions{
			DurationUnit: time.Minute,
			TimeLayouts:  []string{time.DateOnly},
		},
	}

	db := (&DB{Dialect: dialect.Postgres}).
		SetExecutor(executorLayer{std}).
		SetBindConfig(config)
	originalExecutor := db.Executor
	table := db.NewTable("t")
	ctx := t.Context()
	err := db.Transaction(ctx, nil, func(txdb *DB) error {
		if txdb.Dialect != db.Dialect || !reflect.DeepEqual(txdb.BindConfig(), config) {
			t.Fatal("transaction lost DB configuration")
		}
		if tx, ok := AsExecutor[*sql.Tx](txdb); !ok || tx == nil {
			t.Fatal("transaction executor not found")
		}
		if _, ok := AsExecutor[*sql.DB](txdb); ok {
			t.Fatal("transaction resolved to the connection pool")
		}

		nestedCalled := false
		if err := txdb.Transaction(ctx, nil, func(*DB) error {
			nestedCalled = true
			return nil
		}); err == nil || nestedCalled {
			t.Fatal("nested transaction was accepted", err)
		}

		_, err := table.WithDB(txdb).Insert().Columns("value").Values(7).ExecContext(ctx)
		txdb.SetBindConfig(BindConfig{Capacity: 1})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if db.Executor != originalExecutor || !reflect.DeepEqual(db.BindConfig(), config) {
		t.Fatal("transaction changed the original DB")
	}
	if f.begun != 1 || f.committed != 1 || f.rolled != 0 || f.execs != 1 || len(*calls) != 1 {
		t.Fatal("missing or duplicate transaction execution/interception", f, len(*calls))
	}

	call := (*calls)[0]
	if call.method != "exec" || call.ctx != ctx || call.query != f.query ||
		!reflect.DeepEqual(call.args, []any{7}) {
		t.Fatal("transaction SQL was not intercepted", call)
	}
}

// Closing a generic executor must never be used as transaction cleanup: its
// Close method may close a shared connection pool.
type transactionCloseGuard struct {
	Executor
	closes int
}

func (e *transactionCloseGuard) Unwrap() Executor { return e.Executor }
func (e *transactionCloseGuard) Close() error {
	e.closes++
	return errors.New("borrowed executor was closed")
}

func TestDBTransactionDoesNotCloseExecutor(t *testing.T) {
	for _, tc := range []struct {
		name        string
		callbackErr error
		commitErr   error
		beginErr    error
	}{
		{name: "success"},
		{name: "callback error", callbackErr: errors.New("callback failed")},
		{name: "commit error", commitErr: errors.New("commit failed")},
		{name: "begin error", beginErr: errors.New("begin failed")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &txFixture{commitErr: tc.commitErr, beginErr: tc.beginErr}
			pool := sql.OpenDB(txConnector{f})
			t.Cleanup(func() { _ = pool.Close() })
			outer := &transactionCloseGuard{Executor: pool}
			db := &DB{Executor: outer}
			var bound *transactionCloseGuard
			previous := defaultExecutorInterceptor
			t.Cleanup(func() { SetDefaultExecutorInterceptor(previous) })
			SetDefaultExecutorInterceptor(func(e Executor) Executor {
				bound = &transactionCloseGuard{Executor: e}
				return bound
			})

			err := db.Transaction(t.Context(), nil, func(*DB) error {
				return tc.callbackErr
			})

			wantErr := tc.callbackErr
			if tc.beginErr != nil {
				wantErr = tc.beginErr
			} else if wantErr == nil {
				wantErr = tc.commitErr
			}

			if err != wantErr {
				t.Fatalf("got error %v; want %v", err, wantErr)
			}
			if outer.closes != 0 || (bound != nil && bound.closes != 0) {
				t.Fatal("transaction closed a borrowed executor")
			}
			if err := pool.PingContext(t.Context()); err != nil {
				t.Fatal("transaction closed the shared connection pool", err)
			}
		})
	}
}
