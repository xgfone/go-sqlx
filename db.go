// Copyright 2020~2026 xgfone
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package sqlx

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/url"
	"runtime"
	"strings"
	"time"

	"github.com/xgfone/go-sqlx/dialect"
)

// DefaultDB is the default global DB.
var DefaultDB = &DB{Dialect: dialect.MySQL}

// SetConnURLLocation sets the argument "loc" in the connection url if missing.
//
// If loc is nil, use Location instead.
func SetConnURLLocation(connURL string, loc *time.Location) string {
	if loc == nil {
		return connURL
	}

	if index := strings.IndexByte(connURL, '?') + 1; index > 0 {
		query, err := url.ParseQuery(connURL[index:])
		if err == nil && query.Get("loc") == "" {
			query.Set("loc", loc.String())
			return connURL[:index] + query.Encode()
		}
		return connURL
	}

	return fmt.Sprintf("%s?loc=%s", connURL, loc.String())
}

// Config is used to configure the DB.
type Config func(db *sql.DB)

// Opener is used to open a *sql.DB.
type Opener func(driverName, dataSourceName string) (*sql.DB, error)

// DefaultConfigs is the default configs.
var DefaultConfigs = []Config{MaxOpenConns(0), ConnMaxIdleTime(time.Minute * 5)}

// DefaultOpener is used to open a *sql.DB.
var DefaultOpener Opener = sql.Open

// MaxOpenConns returns a Config to set the maximum number of the open connection.
//
// If maxnum is equal to 0, it is runtime.NumCPU()*2 by default.
func MaxOpenConns(maxnum int) Config {
	if maxnum == 0 {
		maxnum = runtime.NumCPU() * 2
	}
	return func(db *sql.DB) { db.SetMaxOpenConns(maxnum) }
}

// MaxIdleConns returns a Config to set the maximum number of the idle connection.
func MaxIdleConns(n int) Config {
	return func(db *sql.DB) { db.SetMaxIdleConns(n) }
}

// ConnMaxLifetime returns a Config to set the maximum lifetime of the connection.
func ConnMaxLifetime(d time.Duration) Config {
	return func(db *sql.DB) { db.SetConnMaxLifetime(d) }
}

// ConnMaxIdleTime returns a Config to set the maximum idle time of the connection.
func ConnMaxIdleTime(d time.Duration) Config {
	return func(db *sql.DB) { db.SetConnMaxIdleTime(d) }
}

// DB is the wrapper of the sql.DB.
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
	v.Executor = e
	return &v
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
