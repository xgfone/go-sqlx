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
	"reflect"
	"time"
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

// Query builds the sql and executes it.
func (b *SelectBuilder) Query() (Rows, error) {
	return b.QueryContext(context.Background())
}

// QueryContext builds the sql and executes it.
func (b *SelectBuilder) QueryContext(ctx context.Context) (Rows, error) {
	query, args := b.Build()
	defer args.Release()
	return b.queryContext(ctx, query, args.Args()...)
}

func (b *SelectBuilder) queryContext(ctx context.Context, rawsql string, args ...any) (Rows, error) {
	rows, err := getDB(b.db).QueryContext(ctx, rawsql, args...)
	return Rows{b.SelectedColumns(), rows}, err
}

// BindRows is equal to b.BindRowsContext(context.Background(), slice).
func (b *SelectBuilder) BindRows(slice any) error {
	return b.BindRowsContext(context.Background(), slice)
}

// BindRowsContext is the same QueryContext, but scans the result set
// into the slice.
//
// Notice: slice must be a pointer to a slice. And the element of the slice
// may be a struct or type implemented the interface sql.Scanner.
func (b *SelectBuilder) BindRowsContext(ctx context.Context, slice any) error {
	rows, err := b.QueryContext(ctx)
	if err != nil {
		return err
	}
	defer rows.Close()
	return rows.ScanSlice(slice)
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

// Scan implements the interface sql.Scanner, which is the proxy of sql.Rows
// and supports that the sql value is NULL.
func (r Rows) Scan(dests ...any) (err error) {
	if r.Rows == nil {
		return
	}

	return ScanRow(r.Rows.Scan, dests...)
}

// ScanStruct is the same as Scan, but the columns are scanned into the struct
// s, which uses ScanColumnsToStruct.
func (r Rows) ScanStruct(s any) (err error) {
	if r.Rows == nil {
		return
	}

	columns := r.Columns
	if len(columns) == 0 {
		if columns, err = r.Rows.Columns(); err != nil {
			return
		}
	}
	return ScanColumnsToStruct(r.Scan, columns, s)
}

// ScanStructWithColumns is the same as Scan, but the columns are scanned
// into the struct s by using ScanColumnsToStruct.
func (r Rows) ScanStructWithColumns(s any, columns ...string) (err error) {
	if r.Rows == nil {
		return
	}

	return ScanColumnsToStruct(r.Scan, columns, s)
}

var scannerType = reflect.TypeOf((*sql.Scanner)(nil)).Elem()

// ScanSlice is used to scan the row set into the slice.
func (r Rows) ScanSlice(slice any) (err error) {
	if r.Rows == nil {
		return
	}

	oldvf := reflect.ValueOf(slice)
	if oldvf.Kind() != reflect.Ptr {
		panic("Rows.ScanSlice: the value must be a pointer to a slice")
	}

	vf := oldvf.Elem()
	if vf.Kind() != reflect.Slice {
		panic("Rows.ScanSlice: the value must be a pointer to a slice")
	}

	vt := vf.Type()
	et := vt.Elem()
	if vf.Cap() == 0 {
		vf.Set(reflect.MakeSlice(vt, 0, DefaultSliceCap))
	}

	scan := r.scansingle
	if et.Kind() == reflect.Struct {
		e := reflect.New(et)
		_, ok := e.Interface().(*time.Time)
		if !ok && !e.Type().Implements(scannerType) {
			scan = r.ScanStruct
		}
	}

	for r.Next() {
		e := reflect.New(et)
		if err := scan(e.Interface()); err != nil {
			return err
		}
		vf = reflect.Append(vf, e.Elem())
	}

	oldvf.Elem().Set(vf)
	return nil
}

func (r Rows) scansingle(v any) error { return r.Scan(v) }
