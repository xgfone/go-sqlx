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
	"errors"
)

// QueryRowOneContext executes raw SQL and returns the library's struct-aware Row.
func (db *DB) QueryRowOneContext(ctx context.Context, query string, args ...any) Row {
	return NewRow(db.queryRowsContext(ctx, nil, query, args...))
}

// QueryRowContext preserves pagination and only narrows its limit to at most one.
func (b *SelectBuilder) QueryRowContext(ctx context.Context) Row {
	q := b.Clone()
	if !q.hasLimit || q.limit > 1 {
		q.Limit(1)
	}
	return b.binder.Row(queryStatement(ctx, q, &q.builderBase))
}

/// ---------------------------------------------------------------------- ///

func (b *binder) Row(rows *sql.Rows, columns []string, err error) Row {
	if b.wrapper == nil {
		return Row{
			rows: rows,
			err:  err,

			columns: columns,
			wrapper: defaultbinder.wrapper,
		}
	}

	return Row{
		rows: rows,
		err:  err,

		columns: columns,
		wrapper: b.wrapper,
	}
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

// Next is always false: Row is a single-use result, not an iterator.
func (r Row) Next() bool { return false }

// Columns returns the names of the selected columns.
func (r Row) Columns() ([]string, error) {
	if r.err != nil {
		return nil, r.err
	}
	if r.rows == nil {
		return nil, errors.New("sqlx: nil row")
	}
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
	if wrapper == nil {
		r.wrapper = defaultbinder.wrapper
	} else {
		r.wrapper = wrapper
	}
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
	defer recoverBinding(&err)
	if r.err != nil {
		return r.err
	}

	if r.rows == nil {
		return errors.New("sqlx: nil row")
	}

	defer func() {
		if e := r.rows.Close(); err == nil {
			err = e
		}
	}()

	if !r.rows.Next() {
		if err := r.rows.Err(); err != nil {
			return err
		}
		return sql.ErrNoRows
	}

	return r.wrapper(newrowscanner(r, r.rows.Scan), dsts...)
}

func (r Row) Err() error {
	if r.err != nil {
		return r.err
	}
	if r.rows == nil {
		return errors.New("sqlx: nil row")
	}
	return r.rows.Err()
}

// Close releases an unread row. Scan and Bind close it automatically.
func (r Row) Close() error {
	if r.rows == nil {
		return nil
	}
	return r.rows.Close()
}
