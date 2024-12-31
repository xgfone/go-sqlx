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
)

var DefaultSliceCap = 16

// QueryRows executes the query sql statement and returns Rows instead of *sql.Rows.
func (db *DB) QueryRows(query string, args ...any) (Rows, error) {
	return db.QueryRowsContext(context.Background(), query, args...)
}

// QueryRowsContext executes the query sql statement and returns Rows instead of *sql.Rows.
func (db *DB) QueryRowsContext(ctx context.Context, query string, args ...any) (rows Rows, err error) {
	if query, args, err = db.Intercept(query, args); err == nil {
		var _rows *sql.Rows
		_rows, err = getDB(db).QueryContext(ctx, query, args...)
		rows.Rows = _rows
	}
	return
}

// Rows is used to wrap sql.Rows.
type Rows struct {
	Columns []string // Only used by ScanStruct
	*sql.Rows
}

// NewRows returns a new Rows.
func NewRows(rows *sql.Rows, columns ...string) Rows {
	return Rows{Rows: rows, Columns: columns}
}

func (r Rows) scan(dests ...any) error {
	return ScanRow(r.Rows.Scan, dests...)
}

func (r Rows) scanStruct(s any) (err error) {
	columns := r.Columns
	if len(columns) == 0 {
		if columns, err = r.Rows.Columns(); err != nil {
			return
		}
	}
	return ScanColumnsToStruct(r.scan, columns, s)
}

// Scan implements the interface sql.Scanner, which is the proxy of sql.Rows
// and supports that the sql value is NULL.
func (r Rows) Scan(dests ...any) (err error) {
	if r.Rows == nil {
		return
	}

	return r.scan(dests...)
}
