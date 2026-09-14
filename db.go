// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"

	"github.com/xgfone/go-sqlx/dialect"
)

// DefaultDB is the default global DB.
var DefaultDB = &DB{Dialect: dialect.MySQL}

// DB is the wrapper of the sql.DB.
//
// Set methods mutate the same DB; configure it before concurrent use
// or synchronize changes with all users.
//
// With methods return a copy with the requested configuration.
type DB struct {
	Dialect
	Executor

	config BindConfig
}

// Open opens a database specified by its database driver name
// and a driver-specific data source name,
func Open(driverName, dataSourceName string, configs ...Config) (*DB, error) {
	d, ok := dialect.Get(driverName)
	if !ok {
		return nil, fmt.Errorf("the dialect '%s' has not been registered",
			driverName)
	}

	db, err := DefaultOpener(driverName, dataSourceName)
	if err != nil {
		return nil, err
	}

	if configs == nil {
		configs = DefaultConfigs
	}
	for _, c := range configs {
		c(db)
	}

	xdb := &DB{Dialect: d, Executor: db}
	return xdb, nil
}

func getDB(db *DB) *DB {
	if db != nil {
		return db
	}
	return DefaultDB
}

func getDialect(db *DB) Dialect {
	if db != nil {
		return resolveDialect(db.Dialect)
	}
	return resolveDialect(nil)
}

// WithExecutor creates a DB with the same dialect and a new executor,
// e.g. a transaction.
func (db *DB) WithExecutor(e Executor) *DB {
	v := *db
	return v.SetExecutor(e)
}

// SetExecutor replaces this DB's executor and returns db. It does not close the
// previous executor. Existing builders without an executor override use the new
// executor on their next execution. See DB for synchronization requirements.
func (db *DB) SetExecutor(e Executor) *DB {
	db.Executor = e
	return db
}

// BeginTx starts a transaction when the executor supports it.
func (db *DB) BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error) {
	if b, ok := db.Executor.(TxBeginner); ok {
		return b.BeginTx(ctx, opts)
	}
	return nil, errors.New("sqlx: executor cannot begin transactions")
}

// Close closes an owning database/connection. Transactions must use Commit/Rollback.
func (db *DB) Close() error {
	if c, ok := db.Executor.(io.Closer); ok {
		return c.Close()
	}
	return errors.New("sqlx: executor cannot be closed")
}
