// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"context"
	"database/sql"
)

var _ Executor = (*sql.DB)(nil)
var _ Executor = (*DB)(nil)
var _ Executor = (*sql.Tx)(nil)
var _ Executor = (*sql.Conn)(nil)
var _ TxBeginner = (*DB)(nil)

// Executor is used to execute the sql statement.
//
// Wrappers may implement Unwrap() [Executor] to expose their inner executor to
// [AsExecutor] and to [DB]'s transaction and close operations.
type Executor interface {
	PrepareContext(ctx context.Context, query string) (*sql.Stmt, error)
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// TxBeginner is used to open a sql transaction.
type TxBeginner interface {
	BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error)
}

var defaultExecutorInterceptor func(Executor) Executor

// SetDefaultExecutorInterceptor sets the interceptor applied by [Open],
// [DB.SetExecutor] (including [DB.WithExecutor]), and each builder's SetExecutor method
// ([SelectBuilder.SetExecutor], [InsertBuilder.SetExecutor],
// [UpdateBuilder.SetExecutor] and [DeleteBuilder.SetExecutor]).
// Passing nil disables interception (the default). Nil executors are skipped.
// Direct assignment to the embedded [Executor] field of [DB] bypasses this hook.
//
// The interceptor must return a usable executor and should preserve the original
// through Unwrap() [Executor]. It may receive an already wrapped executor; use
// [AsExecutor] to avoid installing the same middleware more than once.
//
// Configure this global before concurrent use, or synchronize changes with all
// callers. Changes only affect subsequently attached executors. The interceptor
// and its returned executors must support the callers' concurrency requirements.
func SetDefaultExecutorInterceptor(interceptor func(Executor) Executor) {
	defaultExecutorInterceptor = interceptor
}

func interceptExecutor(e Executor) Executor {
	if e != nil && defaultExecutorInterceptor != nil {
		return defaultExecutorInterceptor(e)
	}
	return e
}

// AsExecutor returns the first value in e's wrapper chain assignable to T,
// starting with e itself and following Unwrap() [Executor]. T may be a concrete
// executor type, such as *[sql.DB], or a capability interface, such as [TxBeginner].
// If no value matches, it returns the zero value of T and false.
//
// Unwrap chains must be finite and acyclic. A matching typed nil is returned
// with true, just as with a type assertion. An outer capability implementation
// takes precedence over an inner one, including when its method returns an error.
func AsExecutor[T any](e Executor) (T, bool) {
	for e != nil {
		if value, ok := e.(T); ok {
			return value, true
		}

		wrapper, ok := e.(interface{ Unwrap() Executor })
		if !ok {
			break
		}

		e = wrapper.Unwrap()
	}

	var zero T
	return zero, false
}
