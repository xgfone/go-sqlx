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

func (db *DB) QueryRowsContext(ctx context.Context, query string, args ...any) Rows {
	return NewRows(db.queryRowsContext(ctx, nil, query, args...))
}

func (db *DB) queryRowsContext(ctx context.Context, columns []string, query string, args ...any) (
	*sql.Rows, []string, error,
) {
	if db == nil || db.Executor == nil {
		return nil, nil, errors.New("sqlx: no executor configured")
	}

	rows, e := db.QueryContext(ctx, query, args...)
	if e != nil {
		return nil, nil, e
	}

	if len(columns) == 0 {
		columns, e = rows.Columns()
		if e != nil {
			_ = rows.Close()
			return nil, nil, e
		}
	}

	return rows, columns, nil
}

func (b *SelectBuilder) QueryRowsContext(ctx context.Context) Rows {
	return b.binder.Rows(queryStatement(ctx, b, &b.builderBase))
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
	v := *b
	if v.rowscap <= 0 {
		v.rowscap = DefaultRowsCap
	}
	if v.wrapper == nil {
		v.wrapper = defaultbinder.wrapper
	}
	if v.binder == nil {
		v.binder = defaultbinder.binder
	}
	return Rows{Rows: rows, err: err, columns: columns, binder: v}
}

// Rows is the same as sql.Rows to scan the rows to a map or slice.
type Rows struct {
	*sql.Rows
	err error

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
	if r.err != nil {
		return nil, r.err
	}
	if r.Rows == nil {
		return nil, errors.New("sqlx: nil rows")
	}
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
	if wrapper == nil {
		r.binder.wrapper = defaultbinder.wrapper
	} else {
		r.binder.wrapper = wrapper
	}
	return r
}

// WithBinder resets the rows binder and returns a new Rows.
func (r Rows) WithBinder(binder RowsBinder) Rows {
	if binder == nil {
		r.binder.binder = defaultbinder.binder
	} else {
		r.binder.binder = binder
	}
	return r
}

// Err returns the error.
func (r Rows) Err() error {
	if r.err != nil {
		return r.err
	}
	if r.Rows == nil {
		return errors.New("sqlx: nil rows")
	}
	return r.Rows.Err()
}

// Bind binds the rows to dst that may be a map or slice
func (r Rows) Bind(dst any) (err error) {
	defer recoverBinding(&err)

	if err := r.Err(); err != nil {
		return err
	}

	defer func() {
		if e := r.Close(); err == nil {
			err = e
		}
	}()

	if err := r.binder.binder.BindRows(r, dst); err != nil {
		return err
	}

	return r.Err()
}

// Scan implements the interface sql.Scanner, which is the same as sql.Rows.Scan
// but supports that the sql value is NULL.
func (r Rows) Scan(dsts ...any) (err error) {
	if e := r.Err(); e != nil {
		return e
	}
	return r.binder.wrapper(newrowscanner(r, r.Rows.Scan), dsts...)
}

func (r Rows) Next() bool {
	return r.err == nil && r.Rows != nil && r.Rows.Next()
}

func (r Rows) Close() error {
	if r.Rows == nil {
		return nil
	}
	return r.Rows.Close()
}
