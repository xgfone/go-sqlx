// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx_test

import (
	"database/sql"
	"fmt"
	"log/slog"

	"github.com/xgfone/go-sqlx"
)

// auditExecutor marks the audit middleware so rebinding does not duplicate it.
type auditExecutor struct{ sqlx.Executor }

func (e *auditExecutor) Unwrap() sqlx.Executor { return e.Executor }

func ExampleDefaultExecutorInterceptor() {
	// Configure this once during application startup, before opening databases.
	previous := sqlx.DefaultExecutorInterceptor
	defer func() { sqlx.DefaultExecutorInterceptor = previous }()
	sqlx.DefaultExecutorInterceptor = func(e sqlx.Executor) sqlx.Executor {
		if _, ok := sqlx.AsExecutor[*auditExecutor](e); ok {
			return e
		}
		return &auditExecutor{
			Executor: sqlx.WrapExecutor(e, func(query string, args []any, err error) {
				slog.Info("sql audit", "query", query, "args", args, "error", err)
			}),
		}
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
