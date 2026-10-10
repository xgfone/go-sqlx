// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync/atomic"

	"github.com/xgfone/go-sqlx/dialect"
)

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

// Open opens a database specified by its database driver name
// and a driver-specific data source name.
//
// If driverName is empty, dataSourceName may have the form
// "[dialect+]driver://dsn". Open removes this prefix and passes dsn unchanged
// to [DefaultOpener]. Without a dialect prefix, driver also names the registered
// dialect. With a dialect prefix, only dialect must be registered in this
// package; driver names the driver used by DefaultOpener.
//
// With no driverName or data source prefix, Open uses [dialect.Default] and its
// Name as the driver name, passing dataSourceName unchanged to DefaultOpener.
// An empty default driver name returns an error.
//
// The remaining dsn must use the driver's native format, including any scheme
// it requires. For example, "postgres+pgx://postgres://user:pass@host/db" selects
// the postgres dialect and pgx driver with "postgres://user:pass@host/db" as dsn.
// A nonempty driverName bypasses prefix parsing.
func Open(driverName, dataSourceName string, configs ...Config) (*DB, error) {
	var d Dialect
	dialectName := driverName
	if driverName == "" {
		prefix, dsn, ok := strings.Cut(dataSourceName, "://")
		// A delimiter inside native credentials, paths, or options is not a
		// metadata prefix. Do not parse or rewrite the native DSN.
		if ok && !strings.ContainsAny(prefix, ":/@?=()\\") {
			dialectName, driverName, ok = strings.Cut(prefix, "+")
			if !ok {
				driverName = dialectName
			}

			if dialectName == "" || driverName == "" ||
				strings.Contains(driverName, "+") ||
				driverName != strings.TrimSpace(driverName) ||
				dialectName != strings.TrimSpace(dialectName) {
				return nil, errors.New("sqlx: invalid [dialect+]driver://dsn prefix")
			}

			dataSourceName = dsn
		} else {
			d = dialect.Default()
			driverName = d.Name()
			if strings.TrimSpace(driverName) == "" {
				return nil, errors.New("sqlx: default dialect has an empty driver name")
			}
		}
	}

	if d == nil {
		d = dialect.Get(dialectName)
		if d == nil {
			return nil, fmt.Errorf("the dialect '%s' has not been registered", dialectName)
		}
	}

	db, err := DefaultOpener(driverName, dataSourceName)
	if err != nil {
		return nil, err
	}

	if configs == nil {
		configs = DefaultConfigs()
	}
	for _, c := range configs {
		c(db)
	}

	xdb := (&DB{Dialect: d}).SetExecutor(db)
	return xdb, nil
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
// Unwrap chain (see [AsExecutor]). Bind the returned *[sql.Tx] with [DB.WithExecutor] or a
// builder method such as
// [SelectBuilder.SetExecutor] to apply the configured interceptor to its SQL execution.
func (db *DB) BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error) {
	if b, ok := AsExecutor[TxBeginner](db.Executor); ok {
		return b.BeginTx(ctx, opts)
	}
	return nil, errors.New("sqlx: executor cannot begin transactions")
}

// Transaction starts a transaction with [DB.BeginTx] and calls f with a
// transaction-bound copy of db, preserving its dialect and binding configuration
// and applying the configured executor interceptor through [DB.WithExecutor].
// A nil opts uses the driver's default transaction options.
//
// If f returns nil, Transaction commits and returns the commit error. Otherwise,
// it attempts to roll back and returns f's error. If f panics, it attempts to
// roll back and lets the panic propagate. Rollback errors are ignored.
//
// All transaction operations must use the DB passed to f; existing Table/Oper
// values must be rebound to it. f must not commit or roll back the transaction
// itself or use the transaction-bound DB after returning. Nested transactions
// are not supported by *[sql.Tx]. Use [DB.BeginTx] for manual transaction control.
func (db *DB) Transaction(ctx context.Context, opts *sql.TxOptions, f func(*DB) error) error {
	tx, err := db.BeginTx(ctx, opts)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	if err := f(db.WithExecutor(tx)); err != nil {
		return err
	}

	return tx.Commit()
}

// Close closes an owning database/connection. Transactions must use
// [sql.Tx.Commit]/[sql.Tx.Rollback].
// It uses the first [io.Closer] in the executor's Unwrap chain (see [AsExecutor]).
func (db *DB) Close() error {
	if c, ok := AsExecutor[io.Closer](db.Executor); ok {
		return c.Close()
	}
	return errors.New("sqlx: executor cannot be closed")
}
