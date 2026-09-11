// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"context"
	"database/sql"
	"reflect"

	"github.com/xgfone/go-sqlx/internal/rowbind"
)

// QueryRowOneContext executes raw SQL and returns a struct-aware single row.
func (db *DB) QueryRowOneContext(ctx context.Context, query string, args ...any) Row {
	return db.binding().row(db.queryRowsContext(ctx, query, args...))
}

// QueryRowContext preserves pagination and narrows its limit to at most one.
func (b *SelectBuilder) QueryRowContext(ctx context.Context) Row {
	// Only pagination scalars are changed. Rendering is read-only, so the
	// query can share its column/condition slices with the caller's builder.
	q := *b
	if q.withTies {
		q.withTies = false
	}
	if !q.hasLimit || q.limit > 1 {
		q.Limit(1)
	}
	return b.binding().row(queryStatement(ctx, &q, &q.builderBase))
}

func (c BindConfig) row(rows *sql.Rows, columns []string, err error) Row {
	return Row{rows: c.rows(rows, columns, err)}
}

// Row owns a single-use rows. Scan and Bind close it automatically. It is not
// an iterator; close an unread Row explicitly to release the connection.
type Row struct{ rows Rows }

func NewRow(rows *sql.Rows, columns []string, err error) Row {
	return Row{rows: NewRows(rows, columns, err)}
}

func (r Row) Columns() ([]string, error) {
	return r.rows.Columns()
}

func (r Row) WithColumns(columns ...string) Row {
	r.rows = r.rows.WithColumns(columns...)
	return r
}

func (r Row) WithScanOptions(options ScanOptions) Row {
	r.rows.config.Scan = cloneScanOptions(options)
	return r
}

// Bind returns false, nil for an empty result. Conversion errors are preserved.
func (r Row) Bind(dst ...any) (bool, error) {
	return CheckErrNoRows(r.Scan(dst...))
}

// Scan uses the same conversion rules as Rows.Scan and returns sql.ErrNoRows
// for an empty result. As with database/sql, row scans are not atomic.
func (r Row) Scan(dst ...any) (err error) {
	defer func() {
		if e := r.Close(); err == nil {
			err = e
		}
	}()

	if err = r.Err(); err != nil {
		return err
	}
	if err = validateScanOptions(r.rows.config.Scan); err != nil {
		return err
	}
	if r.rows.hasLabels() {
		if _, err = r.rows.scanColumns(); err != nil {
			return err
		}
	}

	if !r.rows.Next() {
		if err := r.Err(); err != nil {
			return err
		}
		return sql.ErrNoRows
	}

	if len(dst) == 1 && dst[0] != nil && !rowbind.IsScalarDestination(reflect.TypeOf(dst[0])) {
		return scanSingleStruct(r.rows, dst)
	}
	return rowbind.ScanScalarRow(r.rows.cursor.rows.Scan, dst, r.rows.config.Scan)
}

func (r Row) Err() error   { return r.rows.Err() }
func (r Row) Close() error { return r.rows.Close() }
