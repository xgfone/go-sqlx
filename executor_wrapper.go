// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"context"
	"database/sql"
)

// WrapExecutor returns an executor that calls after synchronously once after
// each underlying Executor method returns. Results and errors are returned
// unchanged. If e or after is nil, it returns e unchanged.
//
// PrepareContext passes nil arguments and reports only statement preparation.
// QueryRowContext reports Row.Err when a row is returned. Errors from later
// Scan, Next, or Close calls, including sql.ErrNoRows, are not observed. Calls
// on a returned *sql.Stmt are not observed either.
//
// The callback borrows the original arguments for its duration; it must not
// modify them and must copy any slice it retains. It must support concurrent
// calls if the executor is used concurrently. Panics propagate; after is not
// called when the underlying method panics.
//
// The wrapper implements Unwrap() Executor for AsExecutor and DB's capability
// lookup. Each WrapExecutor call adds a layer, even to an already wrapped input.
func WrapExecutor(e Executor, after func(query string, args []any, err error)) Executor {
	if e == nil || after == nil {
		return e
	}
	return &afterExecutor{Executor: e, after: after}
}

type afterExecutor struct {
	after func(string, []any, error)
	Executor
}

func (e *afterExecutor) Unwrap() Executor { return e.Executor }

func (e *afterExecutor) PrepareContext(ctx context.Context, query string) (*sql.Stmt, error) {
	stmt, err := e.Executor.PrepareContext(ctx, query)
	e.after(query, nil, err)
	return stmt, err
}

func (e *afterExecutor) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	result, err := e.Executor.ExecContext(ctx, query, args...)
	e.after(query, args, err)
	return result, err
}

func (e *afterExecutor) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	rows, err := e.Executor.QueryContext(ctx, query, args...)
	e.after(query, args, err)
	return rows, err
}

func (e *afterExecutor) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	row := e.Executor.QueryRowContext(ctx, query, args...)
	var err error
	if row != nil {
		err = row.Err()
	}
	e.after(query, args, err)
	return row
}
