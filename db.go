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

// CreateTable returns a SQL table builder.
func (db *DB) CreateTable(table string) *TableBuilder {
	executor := db.Executor
	if executor == nil {
		executor = db.DB
	}

	return Table(table).SetDialect(db.Dialect).SetExecutor(executor).
		SetInterceptor(db.Interceptor)
}

// Delete returns a DELETE SQL builder.
func (db *DB) Delete() *DeleteBuilder {
	executor := db.Executor
	if executor == nil {
		executor = db.DB
	}

	return Delete().SetDialect(db.Dialect).SetExecutor(executor).
		SetInterceptor(db.Interceptor)
}

// Insert returns a INSERT SQL builder.
func (db *DB) Insert() *InsertBuilder {
	executor := db.Executor
	if executor == nil {
		executor = db.DB
	}

	return Insert().SetDialect(db.Dialect).SetExecutor(executor).
		SetInterceptor(db.Interceptor)
}

// Select returns a SELECT SQL builder.
func (db *DB) Select(column string, alias ...string) *SelectBuilder {
	executor := db.Executor
	if executor == nil {
		executor = db.DB
	}

	return Select(column, alias...).SetDialect(db.Dialect).
		SetExecutor(executor).SetInterceptor(db.Interceptor)
}

// Selects is equal to db.Select(columns[0]).Select(columns[1])...
func (db *DB) Selects(columns ...string) *SelectBuilder {
	executor := db.Executor
	if executor == nil {
		executor = db.DB
	}

	return Selects(columns...).SetDialect(db.Dialect).SetExecutor(executor).
		SetInterceptor(db.Interceptor)
}

// SelectStruct is equal to db.Select().SelectStruct(s).
func (db *DB) SelectStruct(s interface{}) *SelectBuilder {
	executor := db.Executor
	if executor == nil {
		executor = db.DB
	}

	return SelectStruct(s).SetDialect(db.Dialect).SetExecutor(executor).
		SetInterceptor(db.Interceptor)
}

// Update returns a UPDATE SQL builder.
func (db *DB) Update(table ...string) *UpdateBuilder {
	executor := db.Executor
	if executor == nil {
		executor = db.DB
	}

	return Update(table...).SetDialect(db.Dialect).SetExecutor(executor).
		SetInterceptor(db.Interceptor)

}
