// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"context"
	"database/sql"
	"reflect"
	"slices"

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
	return Row{
		err:     err,
		rows:    rows,
		columns: slices.Clone(columns),
		options: c.Scan,
	}
}

// Row owns a single-use rows. Scan and Bind close it automatically. It is not
// an iterator; close an unread Row explicitly to release the connection.
type Row struct {
	err     error
	rows    *sql.Rows
	options ScanOptions
	columns []string
	labels  []string
}

// Build a fresh adapter rather than copying Rows or sharing mutable options
// between the value-returning Row.With methods. Row never advances result sets.
func (r Row) result() *Rows {
	return &Rows{
		err:     r.err,
		rows:    r.rows,
		config:  BindConfig{Scan: r.options},
		columns: r.columns,
		labels:  r.labels,
	}
}

func NewRow(rows *sql.Rows, columns []string, err error) Row {
	return Row{rows: rows, labels: slices.Clone(columns), err: err}
}

func (r Row) Columns() ([]string, error) {
	return r.result().Columns()
}

func (r Row) WithColumns(columns ...string) Row {
	r.labels = slices.Clone(columns)
	return r
}

func (r Row) WithScanOptions(options ScanOptions) Row {
	r.options = cloneScanOptions(options)
	return r
}

// Bind returns false, nil for an empty result. Conversion errors are preserved.
func (r Row) Bind(dst ...any) (bool, error) {
	return CheckErrNoRows(r.Scan(dst...))
}

// Scan uses the same conversion rules as Rows.Scan and returns sql.ErrNoRows
// for an empty result. As with database/sql, row scans are not atomic.
// It borrows dst for the call and does not retain the slice after returning.
func (r Row) Scan(dst ...any) (err error) {
	rows := r.result()
	defer func() {
		if e := rows.Close(); err == nil {
			err = e
		}
	}()

	if err = rows.Err(); err != nil {
		return err
	}
	if err = validateScanOptions(r.options); err != nil {
		return err
	}
	if rows.hasLabels() {
		if _, err = rows.scanColumns(); err != nil {
			return err
		}
	}

	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return err
		}
		return sql.ErrNoRows
	}

	if len(dst) == 1 && dst[0] != nil && !rowbind.IsScalarDestination(reflect.TypeOf(dst[0])) {
		return scanSingleStruct(rows, dst)
	}
	return rowbind.ScanScalarRow(r.rows.Scan, dst, r.options)
}

func (r Row) Err() error   { return r.result().Err() }
func (r Row) Close() error { return r.result().Close() }
