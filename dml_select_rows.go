// Copyright 2025 xgfone
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
)

// QueryRows executes the query sql statement and returns Rows instead of *sql.Rows.
func (db *DB) QueryRows(query string, args ...any) Rows {
	return db.QueryRowsContext(context.Background(), query, args...)
}

// QueryRowsContext executes the query sql statement and returns Rows instead of *sql.Rows.
func (db *DB) QueryRowsContext(ctx context.Context, query string, args ...any) Rows {
	return NewRows(db.queryRowsContext(ctx, nil, query, args...))
}

func (db *DB) queryRowsContext(ctx context.Context, columns []string, query string, args ...any) (*sql.Rows, []string, error) {
	query, args, err := db.Intercept(query, args)
	if err != nil {
		return nil, nil, err
	}

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, nil, err
	}

	if len(columns) == 0 {
		if columns, err = rows.Columns(); err != nil {
			_ = rows.Close()
			return nil, nil, err
		}
	}

	return rows, columns, nil
}

// QueryRows builds the sql and executes it.
func (b *SelectBuilder) QueryRows() Rows {
	return b.QueryRowsContext(context.Background())
}

// QueryRowsContext builds the sql and executes it.
func (b *SelectBuilder) QueryRowsContext(ctx context.Context) Rows {
	query, args := b.Build()
	defer args.Release()

	_args := args.Args()
	columns := b.SelectedColumns()
	return b.binder.Rows(getDB(b.db).queryRowsContext(ctx, columns, query, _args...))
}

/// ---------------------------------------------------------------------- ///

var defaultbinder = binder{
	rowscap: DefaultRowsCap,
	wrapper: DefaultRowScanWrapper,
	binder:  DefaultMixRowsBinder,
}

type binder struct {
	rowscap int
	wrapper RowScannerWrapper
	binder  RowsBinder
}

func (b *binder) Rows(rows *sql.Rows, columns []string, err error) Rows {
	if b.rowscap == 0 && b.wrapper == nil && b.binder == nil {
		return Rows{Rows: rows, Err: err, columns: columns, binder: defaultbinder}
	}
	return Rows{Rows: rows, Err: err, columns: columns, binder: *b}
}

// Rows is the same as sql.Rows to scan the rows to a map or slice.
type Rows struct {
	*sql.Rows
	Err error

	columns []string
	binder  binder
}

// NewRows returns a new Rows.
func NewRows(rows *sql.Rows, columns []string, err error) Rows {
	return defaultbinder.Rows(rows, columns, err)
}

// RowsCap returns the capacity of the rows.
func (r Rows) RowsCap() int {
	return r.binder.rowscap
}

// Columns returns the names of the selected columns.
func (r Rows) Columns() ([]string, error) {
	if len(r.columns) > 0 {
		return r.columns, nil
	}
	return r.Rows.Columns()
}

// WithRowsCap resets the capacity of the rows and returns a new Rows.
func (r Rows) WithRowsCap(cap int) Rows {
	r.binder.rowscap = cap
	return r
}

// WithColumns resets the names of the selected columns and returns a new Rows.
func (r Rows) WithColumns(columns ...string) Rows {
	r.columns = columns
	return r
}

// WithScanner resets the row scanner wrapper and returns a new Rows.
func (r Rows) WithScanner(wrapper RowScannerWrapper) Rows {
	r.binder.wrapper = wrapper
	return r
}

// WithBinder resets the rows binder and returns a new Rows.
func (r Rows) WithBinder(binder RowsBinder) Rows {
	r.binder.binder = binder
	return r
}

// Bind binds the rows to dst that may be a map or slice
func (r Rows) Bind(dst any) error {
	if r.Err != nil {
		return r.Err
	}

	defer r.Rows.Close()
	return r.binder.binder.BindRows(r, dst)
}

// Scan implements the interface sql.Scanner, which is the same as sql.Rows.Scan
// but supports that the sql value is NULL.
func (r Rows) Scan(dsts ...any) (err error) {
	return r.binder.wrapper(newrowscanner(r, r.Rows.Scan), dsts...)
}
