// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"database/sql"
	"errors"
	"reflect"

	"github.com/xgfone/go-sqlx/internal/rowbind"
)

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

// WithScan prepares and lends a type-checked current-row scan function to run.
// It validates the result shape before calling run, even for an empty result.
// *Rows supplies its binding labels and ScanOptions; other RowScanners use zero
// ScanOptions. Single-use Row values are rejected; use Row.Scan instead.
//
// Each call covers one result set with fixed labels, options and destination
// types. Destination addresses may change. For *Rows, SetColumns, SetScanOptions,
// SetBindConfig, NextResultSet or Close during run invalidates the scope and
// returns an error. For raw scanners, the caller must keep the result set and
// metadata unchanged. Start a new WithScan call after changing sets or options.
//
// The scan function borrows its destinations only for each call. Use it only
// synchronously within run; calls after run returns fail. Scratch and source
// references are released on return, error or panic; panics propagate. WithScan
// does not advance or close the cursor. The caller owns iteration, Err and Close.
func WithScan(scanner RowScanner, types []reflect.Type, run func(scan RowScanFunc) error) error {
	if run == nil {
		return errors.New("sqlx: nil scan callback")
	}

	source, columns, options, err := scanSource(scanner)
	if err != nil {
		return err
	}

	mapping, err := rowbind.Prepare(columns, types, options)
	if err != nil {
		return err
	}

	if rows, ok := scanner.(*Rows); ok {
		revision := rows.revision
		err := mapping.WithCheckedScan(func(dst ...any) error {
			if rows.revision != revision {
				return errScanScopeChanged
			}
			if err := rows.Err(); err != nil {
				return err
			}
			return rows.rows.Scan(dst...)
		}, run)
		if err == nil && rows.revision != revision {
			return errScanScopeChanged
		}
		return err
	}

	return mapping.WithCheckedScan(source, run)
}

var errScanScopeChanged = errors.New("sqlx: result set or scan configuration changed during WithScan")

// Select the raw function so a prepared scan does not repeat Rows.Scan's
// mapping and adaptation. Only this public convenience API infers configuration.
func scanSource(scanner RowScanner) (RowScanFunc, []string, ScanOptions, error) {
	if nilBindingValue(scanner) {
		return nil, nil, ScanOptions{}, errors.New("sqlx: nil row scanner")
	}
	switch rows := scanner.(type) {
	case *Rows:
		columns, err := rows.scanColumns()
		if err != nil {
			return nil, nil, ScanOptions{}, err
		}
		return rows.rows.Scan, columns, rows.config.ScanOptions, nil

	case Row, *Row:
		return nil, nil, ScanOptions{}, errors.New("sqlx: use Row.Scan for a single-use result")

	default:
		columns, err := scanner.Columns()
		return scanner.Scan, columns, ScanOptions{}, err
	}
}

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

func nilBindingValue(v any) bool {
	if v == nil {
		return true
	}

	switch r := reflect.ValueOf(v); r.Kind() {
	case reflect.Pointer, reflect.Map, reflect.Slice, reflect.Func,
		reflect.Chan, reflect.Interface:
		return r.IsNil()
	}

	return false
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
