// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx_test

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"

	"github.com/xgfone/go-sqlx"
)

// auditExecutor records each Executor operation before forwarding it. Unwrap
// lets sqlx find the underlying database and its optional capabilities.
type auditExecutor struct{ sqlx.Executor }

func (e *auditExecutor) Unwrap() sqlx.Executor { return e.Executor }

func (e *auditExecutor) PrepareContext(ctx context.Context, query string) (*sql.Stmt, error) {
	slog.InfoContext(ctx, "sql audit", "operation", "prepare", "query", query)
	return e.Executor.PrepareContext(ctx, query)
}

func (e *auditExecutor) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	slog.InfoContext(ctx, "sql audit", "operation", "exec", "query", query, "args", args)
	return e.Executor.ExecContext(ctx, query, args...)
}

func (e *auditExecutor) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	slog.InfoContext(ctx, "sql audit", "operation", "query", "query", query, "args", args)
	return e.Executor.QueryContext(ctx, query, args...)
}

func (e *auditExecutor) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	slog.InfoContext(ctx, "sql audit", "operation", "queryrow", "query", query, "args", args)
	return e.Executor.QueryRowContext(ctx, query, args...)
}

func ExampleDefaultExecutorInterceptor() {
	// Configure this once during application startup, before opening databases.
	previous := sqlx.DefaultExecutorInterceptor
	defer func() { sqlx.DefaultExecutorInterceptor = previous }()
	sqlx.DefaultExecutorInterceptor = func(e sqlx.Executor) sqlx.Executor {
		if _, ok := sqlx.AsExecutor[*auditExecutor](e); ok {
			return e
		}
		return &auditExecutor{Executor: e}
	}

	// Open applies the same hook after configuring its underlying *sql.DB.
	// For an existing executor, use SetExecutor or WithExecutor.
	std := new(sql.DB) // Placeholder; use an opened *sql.DB in an application.
	db := new(sqlx.DB).SetExecutor(std)
	_, audited := db.Executor.(*auditExecutor)

	fmt.Println(audited)
	fmt.Println(db.WithExecutor(db.Executor).Executor == db.Executor)
	// Output:
	// true
	// true
}

func ExampleAsExecutor() {
	std := new(sql.DB)
	db := &sqlx.DB{Executor: &auditExecutor{Executor: &auditExecutor{Executor: std}}}
	raw, ok := sqlx.AsExecutor[*sql.DB](db)
	fmt.Println(raw == std, ok)

	// The audit wrapper needs no BeginTx method. DB.BeginTx uses this lookup.
	_, canBegin := sqlx.AsExecutor[sqlx.TxBeginner](db.Executor)
	fmt.Println(canBegin)
	// Output:
	// true true
	// true
}
