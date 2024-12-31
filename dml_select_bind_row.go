// Copyright 2023 xgfone
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
	query, args, err := db.Intercept(query, args)
	if err != nil {
		panic(err)
	}
	return Row{Row: getDB(db).QueryRowContext(ctx, query, args...)}
}

// BindRow is equal to b.BindRowContext(context.Background(), dest...).
func (b *SelectBuilder) BindRow(dest ...any) (bool, error) {
	return b.BindRowContext(context.Background(), dest...)
}

// BindRowStruct is equal to b.BindRowStructContext(context.Background(), dest).
func (b *SelectBuilder) BindRowStruct(dest any) (bool, error) {
	return b.BindRowStructContext(context.Background(), dest)
}

// BindRowContext is convenient function, which is equal to
// b.QueryRowContext(c).Bind(dest...).
func (b *SelectBuilder) BindRowContext(c context.Context, dest ...any) (bool, error) {
	return b.QueryRowContext(c).Bind(dest...)
}

// BindRowStructContext is convenient function, which is equal to
// b.QueryRowContext(c).BindStruct(dest).
func (b *SelectBuilder) BindRowStructContext(c context.Context, dest any) (bool, error) {
	return b.QueryRowContext(c).BindStruct(dest)
}

// QueryRow builds the sql and executes it.
func (b *SelectBuilder) QueryRow() Row {
	return b.QueryRowContext(context.Background())
}

// QueryRowContext builds the sql and executes it.
func (b *SelectBuilder) QueryRowContext(ctx context.Context) Row {
	query, args := b.Limit(1).Build()
	defer args.Release()
	return b.queryRowContext(ctx, query, args.Args()...)
}

func (b *SelectBuilder) queryRowContext(ctx context.Context, rawsql string, args ...any) Row {
	return Row{b.SelectedColumns(), getDB(b.db).QueryRowContext(ctx, rawsql, args...)}
}

// Row is used to wrap sql.Row.
type Row struct {
	Columns []string // Only used by ScanStruct
	*sql.Row
}

// NewRow returns a new Row.
func NewRow(row *sql.Row, columns ...string) Row {
	return Row{Row: row, Columns: columns}
}

// Bind is the same as Scan, but returns (false, nil) if Scan returns sql.ErrNoRows.
func (r Row) Bind(dests ...any) (ok bool, err error) {
	err = r.Scan(dests...)
	ok, err = CheckErrNoRows(err)
	return
}

// Bind is the same as BindStruct, but returns (false, nil) if Scan returns sql.ErrNoRows.
func (r Row) BindStruct(s any) (ok bool, err error) {
	err = r.ScanStruct(s)
	ok, err = CheckErrNoRows(err)
	return
}

// Scan implements the interface sql.Scanner, which is the proxy of sql.Row
// and supports that the sql value is NULL.
func (r Row) Scan(dests ...any) (err error) {
	return ScanRow(r.Row.Scan, dests...)
}

// ScanStruct is the same as Scan, but the columns are scanned into the struct
// s, which uses ScanColumnsToStruct.
func (r Row) ScanStruct(s any) (err error) {
	return ScanColumnsToStruct(r.Scan, r.Columns, s)
}

// ScanStructWithColumns is the same as Scan, but the columns are scanned
// into the struct s by using ScanColumnsToStruct.
func (r Row) ScanStructWithColumns(s any, columns ...string) (err error) {
	return ScanColumnsToStruct(r.Scan, columns, s)
}
