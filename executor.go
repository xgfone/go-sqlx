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
