// Copyright 2020~2023 xgfone
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
	"fmt"
	"net/url"
	"runtime"
	"strings"
	"time"

	"github.com/xgfone/go-defaults"
)

// DefaultDB is the default global DB.
var DefaultDB = new(DB)

// SetConnURLLocation sets the argument "loc" in the connection url if missing.
//
// If loc is nil, use Location instead.
func SetConnURLLocation(connURL string, loc *time.Location) string {
	if loc == nil {
		if loc = defaults.TimeLocation.Get(); loc == nil {
			return connURL
		}
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
type Config func(*DB)

// Opener is used to open a *sql.DB.
type Opener func(driverName, dataSourceName string) (*sql.DB, error)

// DefaultConfigs is the default configs.
var DefaultConfigs = []Config{MaxOpenConns(0)}

// DefaultOpener is used to open a *sql.DB.
var DefaultOpener Opener = sql.Open

// MaxOpenConns returns a Config to set the maximum number of the open connection.
//
// If maxnum is equal to or less than 0, it is runtime.NumCPU()*2 by default.
func MaxOpenConns(maxnum int) Config {
	if maxnum <= 0 {
		maxnum = runtime.NumCPU() * 2
	}
	return func(db *DB) { db.SetMaxOpenConns(maxnum) }
}

// MaxIdleConns returns a Config to set the maximum number of the idle connection.
func MaxIdleConns(n int) Config {
	return func(db *DB) { db.SetMaxIdleConns(n) }
}

// ConnMaxLifetime returns a Config to set the maximum lifetime of the connection.
func ConnMaxLifetime(d time.Duration) Config {
	return func(db *DB) { db.SetConnMaxLifetime(d) }
}

// DB is the wrapper of the sql.DB.
type DB struct {
	*sql.DB
	Dialect
	Executor
	Interceptor
}

// Open opens a database specified by its database driver name
// and a driver-specific data source name,
func Open(driverName, dataSourceName string, configs ...Config) (*DB, error) {
	dialect := GetDialect(driverName)
	if dialect == nil {
		return nil, fmt.Errorf("the dialect '%s' has not been registered",
			driverName)
	}

	db, err := DefaultOpener(driverName, dataSourceName)
	if err != nil {
		return nil, err
	}

	xdb := &DB{Dialect: dialect, DB: db}
	if configs == nil {
		configs = DefaultConfigs
	}
	for _, c := range configs {
		c(xdb)
	}

	return xdb, nil
}

func getExecutor(db *DB, executor Executor) Executor {
	if executor != nil {
		return executor
	}
	if db != nil {
		return db
	}
	return DefaultDB
}

func getDialect(db *DB, dialect Dialect) Dialect {
	if dialect != nil {
		return dialect
	}
	if db != nil && db.Dialect != nil {
		return db.Dialect
	}
	return DefaultDialect
}

func getInterceptor(db *DB, interceptor Interceptor) Interceptor {
	if interceptor != nil {
		return interceptor
	}
	if db != nil {
		return db.Interceptor
	}
	return nil
}

// GetExecutor returns the executor if set. Or, return sql.DB instead.
func (db *DB) GetExecutor() Executor {
	if db.Executor == nil {
		return db.DB
	}
	return db.Executor
}

// CreateTable returns a SQL table builder.
func (db *DB) CreateTable(table string) *TableBuilder {
	return NewTableBuilder(table).SetDB(db)
}

// Delete returns a DELETE SQL builder.
func (db *DB) Delete(tables ...string) *DeleteBuilder {
	return Delete(tables...).SetDB(db)
}

// Insert returns a INSERT SQL builder.
func (db *DB) Insert() *InsertBuilder {
	return Insert().SetDB(db)
}

// Select returns a SELECT SQL builder.
func (db *DB) Select(column string, alias ...string) *SelectBuilder {
	return Select(column, alias...).SetDB(db)
}

// Selects is equal to db.Select(columns[0]).Select(columns[1])...
func (db *DB) Selects(columns ...string) *SelectBuilder {
	return Selects(columns...).SetDB(db)
}

// SelectColumns is equal to db.Select(columns[0].FullName()).Select(columns[1].FullName())...
func (db *DB) SelectColumns(columns ...Column) *SelectBuilder {
	return SelectColumns(columns...).SetDB(db)
}

// SelectStruct is equal to db.Select().SelectStruct(s, table...).
func (db *DB) SelectStruct(s interface{}, table ...string) *SelectBuilder {
	return SelectStruct(s, table...).SetDB(db)
}

// Update returns a UPDATE SQL builder.
func (db *DB) Update(table ...string) *UpdateBuilder {
	return Update(table...).SetDB(db)
}

// ExecContext executes the sql statement.
func (db *DB) ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	return db.GetExecutor().ExecContext(ctx, query, args...)
}

// QueryContext executes the query sql statement.
func (db *DB) QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	return db.GetExecutor().QueryContext(ctx, query, args...)
}

// QueryRowContext executes the row query sql statement.
func (db *DB) QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row {
	return db.GetExecutor().QueryRowContext(ctx, query, args...)
}
