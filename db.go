// Copyright 2020 xgfone
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
	"time"
)

// DatetimeLayout is the time layout format of SQL DATETIME
const DatetimeLayout = "2006-01-02 15:04:05"

// Location is used to save the default location of time.Time.
var Location = time.Local

// DB is the wrapper of the sql.DB.
type DB struct {
	*sql.DB
	Dialect
	Executor
	Interceptor
}

// Open opens a database specified by its database driver name
// and a driver-specific data source name,
func Open(driverName, dataSourceName string) (*DB, error) {
	dialect := GetDialect(driverName)
	if dialect == nil {
		return nil, fmt.Errorf("the dialect '%s' has not been registered",
			driverName)
	}

	db, err := sql.Open(driverName, dataSourceName)
	if err != nil {
		return nil, err
	}
	return &DB{Dialect: dialect, DB: db}, nil
}

func (db *DB) getExecutor() Executor {
	if db.Executor == nil {
		return db.DB
	}
	return db.Executor
}

// CreateTable returns a SQL table builder.
func (db *DB) CreateTable(table string) *TableBuilder {
	return Table(table).SetDialect(db.Dialect).SetExecutor(db.getExecutor()).
		SetInterceptor(db.Interceptor)
}

// Delete returns a DELETE SQL builder.
func (db *DB) Delete(tables ...string) *DeleteBuilder {
	return Delete(tables...).SetDialect(db.Dialect).SetExecutor(db.getExecutor()).
		SetInterceptor(db.Interceptor)
}

// Insert returns a INSERT SQL builder.
func (db *DB) Insert() *InsertBuilder {
	return Insert().SetDialect(db.Dialect).SetExecutor(db.getExecutor()).
		SetInterceptor(db.Interceptor)
}

// Select returns a SELECT SQL builder.
func (db *DB) Select(column string, alias ...string) *SelectBuilder {
	return Select(column, alias...).SetDialect(db.Dialect).
		SetExecutor(db.getExecutor()).SetInterceptor(db.Interceptor)
}

// Selects is equal to db.Select(columns[0]).Select(columns[1])...
func (db *DB) Selects(columns ...string) *SelectBuilder {
	return Selects(columns...).SetDialect(db.Dialect).SetExecutor(db.getExecutor()).
		SetInterceptor(db.Interceptor)
}

// SelectColumns is equal to db.Select(columns[0].Name()).Select(columns[1].Name())...
func (db *DB) SelectColumns(columns ...Column) *SelectBuilder {
	return SelectColumns(columns...).SetDialect(db.Dialect).
		SetExecutor(db.getExecutor()).SetInterceptor(db.Interceptor)
}

// SelectStruct is equal to db.Select().SelectStruct(s, table...).
func (db *DB) SelectStruct(s interface{}, table ...string) *SelectBuilder {
	return SelectStruct(s, table...).SetDialect(db.Dialect).SetExecutor(db.getExecutor()).
		SetInterceptor(db.Interceptor)
}

// Update returns a UPDATE SQL builder.
func (db *DB) Update(table ...string) *UpdateBuilder {
	return Update(table...).SetDialect(db.Dialect).SetExecutor(db.getExecutor()).
		SetInterceptor(db.Interceptor)
}

// ExecContext executes the sql statement.
func (db *DB) ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	return db.getExecutor().ExecContext(ctx, query, args...)
}

// QueryContext executes the query sql statement.
func (db *DB) QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	return db.getExecutor().QueryContext(ctx, query, args...)
}

// QueryRowContext executes the row query sql statement.
func (db *DB) QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row {
	return db.getExecutor().QueryRowContext(ctx, query, args...)
}
