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

// ConnMaxIdleTime returns a Config to set the maximum idle time of the connection.
func ConnMaxIdleTime(d time.Duration) Config {
	return func(db *DB) { db.SetConnMaxIdleTime(d) }
}

// DB is the wrapper of the sql.DB.
type DB struct {
	*sql.DB
	Dialect
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

func getDB(db *DB) *DB {
	if db != nil {
		return db
	}
	return DefaultDB
}

// GetDialect returns the dialect of the db.
//
// If not set, return DefaultDialect instead.
func (db *DB) GetDialect() Dialect {
	if db != nil && db.Dialect != nil {
		return db.Dialect
	}
	return DefaultDialect
}

func (db *DB) Intercept(sql string, args []interface{}) (string, []interface{}, error) {
	if db != nil && db.Interceptor != nil {
		var err error
		if sql, args, err = db.Interceptor.Intercept(sql, args); err != nil {
			return "", nil, err
		}
	}
	return sql, args, nil
}

// Exec is equal to db.ExecContext(context.Background(), query, args...).
func (db *DB) Exec(ctx context.Context, query string, args ...interface{}) (r sql.Result, err error) {
	return db.ExecContext(context.Background(), query, args...)
}

// Query is equal to db.QueryContext(context.Background(), query, args...).
func (db *DB) Query(ctx context.Context, query string, args ...interface{}) (rows *sql.Rows, err error) {
	return db.QueryContext(context.Background(), query, args...)
}

// QueryRow is equal to db.QueryRowContext(context.Background(), query, args...)
func (db *DB) QueryRow(ctx context.Context, query string, args ...interface{}) *sql.Row {
	return db.QueryRowContext(context.Background(), query, args...)
}

// ExecContext executes the sql statement.
func (db *DB) ExecContext(ctx context.Context, query string, args ...interface{}) (r sql.Result, err error) {
	if query, args, err = db.Intercept(query, args); err == nil {
		r, err = db.DB.ExecContext(ctx, query, args...)
	}
	return
}

// QueryContext executes the query sql statement.
func (db *DB) QueryContext(ctx context.Context, query string, args ...interface{}) (rows *sql.Rows, err error) {
	if query, args, err = db.Intercept(query, args); err == nil {
		rows, err = db.DB.QueryContext(ctx, query, args...)
	}
	return
}

// QueryRowContext executes the row query sql statement.
func (db *DB) QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row {
	query, args, err := db.Intercept(query, args)
	if err != nil {
		panic(err)
	}
	return db.DB.QueryRowContext(ctx, query, args...)
}
