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
)

type wrapperResultExecutor struct {
	stmt   *sql.Stmt
	result sql.Result
	rows   *sql.Rows
	row    *sql.Row
	err    error
	done   bool
}

func (e *wrapperResultExecutor) PrepareContext(context.Context, string) (*sql.Stmt, error) {
	e.done = true
	return e.stmt, e.err
}

func (e *wrapperResultExecutor) ExecContext(context.Context, string, ...any) (sql.Result, error) {
	e.done = true
	return e.result, e.err
}

func (e *wrapperResultExecutor) QueryContext(context.Context, string, ...any) (*sql.Rows, error) {
	e.done = true
	return e.rows, e.err
}

func (e *wrapperResultExecutor) QueryRowContext(context.Context, string, ...any) *sql.Row {
	e.done = true
	return e.row
}

func TestWrapExecutorForwarding(t *testing.T) {
	ctx := t.Context()
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	errorRow := bindTestDB(t, &bindFixture{}).QueryRowContext(canceled, "q")

	for _, failure := range []error{nil, context.Canceled} {
		base := &wrapperResultExecutor{
			stmt: new(sql.Stmt), result: driver.RowsAffected(7),
			rows: new(sql.Rows), row: new(sql.Row), err: failure,
		}

		outcome := "success"
		if failure != nil {
			base.row = errorRow
			outcome = "error"
		}

		query, args := "SELECT ?", []any{sql.Named("value", 7)}
		for _, tc := range []struct {
			method string
			result any
			run    func(Executor) (any, error)
		}{
			{"prepare", base.stmt, func(e Executor) (any, error) { return e.PrepareContext(ctx, query) }},
			{"exec", base.result, func(e Executor) (any, error) { return e.ExecContext(ctx, query, args...) }},
			{"query", base.rows, func(e Executor) (any, error) { return e.QueryContext(ctx, query, args...) }},
			{"queryrow", base.row, func(e Executor) (any, error) {
				row := e.QueryRowContext(ctx, query, args...)
				return row, row.Err()
			}},
		} {
			t.Run(tc.method+"/"+outcome, func(t *testing.T) {
				base.done = false
				var calls []interceptedCall
				inner := &interceptedExecutor{executorLayer{base}, &calls}
				wantArgs := args
				if tc.method == "prepare" {
					wantArgs = nil
				}

				afterCalls := 0
				e := WrapExecutor(inner, func(q string, a []any, err error) {
					afterCalls++
					if !base.done || q != query || !reflect.DeepEqual(a, wantArgs) || err != failure {
						t.Fatal("callback ran early or received changed arguments", base.done, q, a, err)
					}
					if len(a) != 0 && &a[0] != &args[0] {
						t.Fatal("callback did not receive the original argument slice")
					}
				})

				result, err := tc.run(e)
				if result != tc.result || err != failure || afterCalls != 1 {
					t.Fatal("result, error, or callback count changed", result, err, afterCalls)
				}

				wantCalls := []interceptedCall{{tc.method, ctx, query, wantArgs}}
				if !reflect.DeepEqual(calls, wantCalls) {
					t.Fatal("underlying executor received changed arguments", calls)
				}
			})
		}
	}
}

func TestWrapExecutorQueryRowScanBoundary(t *testing.T) {
	for _, tc := range []struct {
		name    string
		values  [][]driver.Value
		scanErr bool
	}{
		{"success", [][]driver.Value{{int64(7)}}, false},
		{"no rows", nil, true},
		{"conversion error", [][]driver.Value{{"not an integer"}}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := &bindFixture{columns: []string{"value"}, values: tc.values}
			std := bindTestDB(t, fixture).Executor
			calls := 0
			e := WrapExecutor(std, func(_ string, _ []any, err error) {
				calls++
				if err != nil || fixture.next.Load() != 0 || fixture.closed.Load() != 0 {
					t.Fatal("callback observed or consumed results before Scan", err)
				}
			})

			var value int
			err := e.QueryRowContext(t.Context(), "SELECT value").Scan(&value)
			if (err != nil) != tc.scanErr || calls != 1 {
				t.Fatal("Scan result or callback count changed", err, calls)
			}
			if tc.values == nil && !errors.Is(err, sql.ErrNoRows) {
				t.Fatal("empty result error changed", err)
			}
			if !tc.scanErr && value != 7 {
				t.Fatal("row was consumed before Scan", value)
			}
		})
	}
}

func TestWrapExecutorUnwrapAndNil(t *testing.T) {
	std := new(sql.DB)
	after := func(string, []any, error) {}

	if WrapExecutor(std, nil) != std || WrapExecutor(nil, after) != nil {
		t.Fatal("nil input or callback changed executor identity")
	}

	e := WrapExecutor(WrapExecutor(std, after), after)
	if got, ok := AsExecutor[*sql.DB](e); !ok || got != std {
		t.Fatal("wrapper lost its original executor")
	}
}
