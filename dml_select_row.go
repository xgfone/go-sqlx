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

// QueryRowOne executes the row query sql statement and returns Row instead of *sql.Row.
func (db *DB) QueryRowOne(query string, args ...any) Row {
	return db.QueryRowOneContext(context.Background(), query, args...)
}

// QueryRowOneContext executes the row query sql statement and returns Row instead of *sql.Row.
func (db *DB) QueryRowOneContext(ctx context.Context, query string, args ...any) Row {
	return NewRow(db.queryRowsContext(ctx, nil, query, args...))
}

// QueryRow builds the sql and executes it.
func (b *SelectBuilder) QueryRow() Row {
	return b.QueryRowContext(context.Background())
}

// QueryRowContext builds the sql and executes it.
func (b *SelectBuilder) QueryRowContext(ctx context.Context) Row {
	query, args := b.Limit(1).Build()
	defer args.Release()

	_args := args.Args()
	columns := b.SelectedColumns()
	return b.binder.Row(getDB(b.db).queryRowsContext(ctx, columns, query, _args...))
}

/// ---------------------------------------------------------------------- ///

func (b *binder) Row(rows *sql.Rows, columns []string, err error) Row {
	if b.wrapper == nil {
		return Row{rows: rows, err: err, columns: columns, wrapper: defaultbinder.wrapper}
	}
	return Row{rows: rows, err: err, columns: columns, wrapper: b.wrapper}
}

// Row is the same as sql.Row to scan the row to the values.
type Row struct {
	rows *sql.Rows
	err  error

	columns []string
	wrapper RowScannerWrapper
}

// NewRow returns a new Row.
func NewRow(rows *sql.Rows, columns []string, err error) Row {
	return defaultbinder.Row(rows, columns, err)
}

// Next is the same as sql.Row.Next, but only used to implement RowScanner and must not be called.
func (r Row) Next() bool { panic("sqlx.Row.Next: cannot be called") }

// Columns returns the names of the selected columns.
func (r Row) Columns() ([]string, error) {
	if len(r.columns) > 0 {
		return r.columns, nil
	}
	return r.rows.Columns()
}

// WithColumns resets the names of the selected columns and returns a new Row.
func (r Row) WithColumns(columns ...string) Row {
	r.columns = columns
	return r
}

// WithScanner resets the row scanner wrapper and returns a new Row.
func (r Row) WithScanner(wrapper RowScannerWrapper) Row {
	r.wrapper = wrapper
	return r
}

// Bind binds the row to the dsts, which never return sql.ErrNoRows as err and uses ok instead of it.
func (r Row) Bind(dsts ...any) (ok bool, err error) {
	err = r.Scan(dsts...)
	ok, err = CheckErrNoRows(err)
	return
}

// Scan implements the interface sql.Scanner, which is the same as sql.Row.Scan
// but supports that the sql value is NULL.
func (r Row) Scan(dsts ...any) (err error) {
	if r.err != nil {
		return r.err
	}
	defer r.rows.Close()

	if !r.rows.Next() {
		if err := r.rows.Err(); err != nil {
			return err
		}
		return sql.ErrNoRows
	}

	return r.wrapper(newrowscanner(r, r.rows.Scan), dsts...)
}
