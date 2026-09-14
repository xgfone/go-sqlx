// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"database/sql"

	"github.com/xgfone/go-sqlx/internal/rowbind"
)

// GeneralScanner adapts scalar values with SQL NULL mapped to the destination's
// zero value (or rejected with NullError). Pointer chains use the same conversion
// rules and remain nil on NULL. Results that retain driver bytes own their
// storage; numeric conversions consume bytes without retaining them. It checks
// numeric ranges and leaves destinations unchanged on conversion errors. Custom
// sql.Scanner values receive the original source, including NULL, and must copy
// borrowed bytes they retain. Their implementations must return errors instead
// of panicking and release their own resources; sqlx does not recover their
// panics or close application Scanners. A nil Value discards the column.
//
// Text numbers use decimal syntax; empty text is invalid. Boolean inputs accept
// ParseBool text, numeric 0/1, and the single binary bytes 0/1. Integer conversions
// reject fractions and overflow. Floating-point conversions allow normal IEEE
// rounding, but reject non-finite values, overflow, and float32 narrowing
// underflow to zero.
//
// Named scalar types and *[]byte are supported in addition to built-in pointers.
// Time strings default to RFC3339Nano, SQL datetime, or SQL date. Existing
// time.Time values retain their location unless Location is explicitly set.
// Numeric timestamps are Unix seconds. Numeric durations use DurationUnit
// (milliseconds by default), regardless of whether the source is integral.
type GeneralScanner = rowbind.GeneralScanner

// RowScanner exposes column metadata and row scanning without iteration methods.
// Row.Scan reads its single row; Rows.Scan scans the current iterator row.
// Scan borrows dst only for the call. Implementations must clone dst before
// retaining the slice or any subslice after returning.
type RowScanner interface {
	Columns() ([]string, error)
	Scan(...any) error
}

// RowScanFunc scans one row into the supplied destinations. The source function
// determines whether scanning is positional or applies struct mapping.
// It borrows the destination slice only for the call; retaining the slice or a
// subslice requires a copy. Temporary scanner adapters must be used synchronously.
// Functions supplied by WithScan and RowMapping.WithScan are valid only within
// their callbacks.
type RowScanFunc = rowbind.RowScanFunc

// RowCursor supplies Next, Err and raw positional Scan. Scan must write source
// column values into the supplied destinations, honoring sql.Scanner, without
// applying another sqlx struct mapping or ScanOptions conversion layer. In
// particular, do not pass *Rows as a raw cursor: its Scan applies those policies
// even though its method set satisfies this interface. Rows.Bind passes *sql.Rows.
//
// Scan borrows its argument slice; retaining it or a subslice requires a copy
// such as slices.Clone(dst). Adapter scanners must be consumed synchronously:
// cloning the slice does not extend their lifetime. Column metadata and policies
// belong to BindOptions at preparation time. The caller owns cursor closing.
type RowCursor = rowbind.Cursor

var (
	_ RowScanner = Row{}
	_ RowScanner = (*Rows)(nil)
	_ RowScanner = (*sql.Rows)(nil)
	_ RowCursor  = (*sql.Rows)(nil)
)

// ScanRow adapts positional scalar destinations, including pointer chains.
// For repeated scanning or struct mapping use WithScan or [Rows.Scan].
// The callback borrows its destination slice and must clone it before retaining
// it or a subslice. Temporary scanner adapters are valid only during the call.
func ScanRow(scan RowScanFunc, dst ...any) error {
	return rowbind.ScanScalarRow(scan, dst, ScanOptions{})
}

func scanSingleStruct(rows *Rows, dst []any) error {
	columns, err := rows.scanColumns()
	if err != nil {
		return err
	}
	return rowbind.ScanStruct(rows.rows.Scan, columns, dst, rows.config.ScanOptions)
}

// ScanColumnsToStruct is a low-level field mapper: it supplies field addresses
// to scan without adapting scalar conversions. Unknown/duplicate columns are
// errors. Use [Rows.Scan] or [WithScan] for conversion policies and cached
// plans. The callback borrows its destination slice only for the call; it must
// use slices.Clone(dst) or an equivalent copy before retaining it or a subslice.
// The cloned entries still point to the caller's fields. Internal scratch is
// released automatically on success, error or panic.
func ScanColumnsToStruct(scan RowScanFunc, columns []string, dst any) error {
	return rowbind.ScanColumnsToStruct(scan, columns, dst)
}
