// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"sync/atomic"
)

// ErrTxUnsupported is returned by [DB.BeginTx] and [DB.Transaction] when no
// executor in the Unwrap chain implements [TxBeginner].
var ErrTxUnsupported = errors.New("sqlx: executor cannot begin transactions")

// ErrCloseUnsupported is returned by [DB.Close] when no executor in the Unwrap
// chain implements [io.Closer].
var ErrCloseUnsupported = errors.New("sqlx: executor cannot be closed")

var defaultDB atomic.Value

func init() {
	SetDefaultDB(&DB{})
}

// DefaultDB returns the default global [DB], initially an empty DB whose nil
// Dialect uses [dialect.Default] at build time. It is never nil.
//
// DefaultDB and [SetDefaultDB] are safe for concurrent use. The returned DB is
// shared; see [DB] for synchronization requirements when changing its fields.
func DefaultDB() *DB {
	return defaultDB.Load().(*DB)
}

// SetDefaultDB sets the DB returned by [DefaultDB]. It panics if db is nil,
// leaving the default unchanged. Existing builders bound to a DB retain it;
// unbound builders use the current default on their next lookup.
//
// SetDefaultDB and [DefaultDB] are safe for concurrent use.
func SetDefaultDB(db *DB) {
	if db == nil {
		panic("sqlx: nil default DB")
	}
	defaultDB.Store(db)
}

// DB is the wrapper of the [sql.DB].
// A nil Dialect uses [dialect.Default] at build time.
//
// Set methods mutate the same [DB]; configure it before concurrent use
// or synchronize changes with all users.
//
// With methods return a copy with the requested configuration.
type DB struct {
	Dialect
	Executor

	config BindConfig
}

func getDB(db *DB) *DB {
	if db != nil {
		return db
	}
	return DefaultDB()
}

func getDialect(db *DB) Dialect {
	if db = getDB(db); db != nil {
		return resolveDialect(db.Dialect)
	}
	return resolveDialect(nil)
}

// WithExecutor creates a [DB] with the same dialect and a new executor,
// e.g. a transaction.
func (db *DB) WithExecutor(e Executor) *DB {
	v := *db
	return v.SetExecutor(e)
}

// SetExecutor replaces this [DB]'s executor and returns db. It does not close the
// previous executor. Existing builders without an executor override use the new
// executor on their next execution. See [DB] for synchronization requirements.
// Non-nil executors use the interceptor set by [SetDefaultExecutorInterceptor].
func (db *DB) SetExecutor(e Executor) *DB {
	db.Executor = interceptExecutor(e)
	return db
}

// Unwrap returns this [DB]'s executor, or nil for a nil [DB].
func (db *DB) Unwrap() Executor {
	if db == nil {
		return nil
	}
	return db.Executor
}

// BeginTx starts a transaction using the first [TxBeginner] in the executor's
// Unwrap chain (see [AsExecutor]). Bind the returned *[sql.Tx] with [DB.WithExecutor]
// or a builder method such as [SelectBuilder.SetExecutor] to apply the configured
// interceptor to its SQL execution. If no executor implements TxBeginner, it returns
// [ErrTxUnsupported]. Errors from the selected TxBeginner are returned unchanged.
func (db *DB) BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error) {
	if b, ok := AsExecutor[TxBeginner](db.Executor); ok {
		return b.BeginTx(ctx, opts)
	}
	return nil, ErrTxUnsupported
}

// Transaction starts a transaction with [DB.BeginTx] and calls f with a
// transaction-bound copy of db, preserving its dialect and binding configuration
// and applying the configured executor interceptor through [DB.WithExecutor].
// A nil opts uses the driver's default transaction options.
//
// If f returns nil, Transaction commits and returns the commit error. Otherwise,
// it attempts to roll back and returns f's error. If f panics, it attempts to
// roll back and lets the original panic propagate, even if rollback fails.
// A rollback error other than [sql.ErrTxDone] is wrapped with rollback context
// and joined with the returned error; [errors.Is]/[errors.As] can inspect both.
// When rollback succeeds or reports [sql.ErrTxDone], the original callback or
// commit error is returned unchanged. Transaction never closes db's executor.
//
// All transaction operations must use the DB passed to f; existing Table/Oper
// values must be rebound to it. f must not commit or roll back the transaction
// itself or use the transaction-bound DB after returning. Nested transactions
// are not supported by *[sql.Tx]. Use [DB.BeginTx] for manual transaction control.
func (db *DB) Transaction(ctx context.Context, opts *sql.TxOptions, f func(*DB) error) (err error) {
	tx, err := db.BeginTx(ctx, opts)
	if err != nil {
		return err
	}

	defer func() {
		rollbackErr := tx.Rollback()
		if rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
			err = errors.Join(err, fmt.Errorf("sqlx: rollback transaction: %w", rollbackErr))
		}
	}()

	if err = f(db.WithExecutor(tx)); err != nil {
		return
	}

	return tx.Commit()
}

// Close closes a caller-owned database/connection. Call it only when the caller
// owns that resource; wrapping an executor does not transfer ownership.
// Transactions  must use [sql.Tx.Commit]/[sql.Tx.Rollback]. It uses the first
// [io.Closer] in the executor's Unwrap chain (see [AsExecutor]). If none is found,
// it returns [ErrCloseUnsupported]. Errors from the selected Closer are returned
// unchanged.
func (db *DB) Close() error {
	if c, ok := AsExecutor[io.Closer](db.Executor); ok {
		return c.Close()
	}
	return ErrCloseUnsupported
}
