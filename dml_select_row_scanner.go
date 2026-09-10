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
type RowScanner interface {
	Columns() ([]string, error)
	Scan(...any) error
}

// RowsScanner is a forward-only result iterator. Row deliberately does not
// implement this interface. Callers own Close when passing an iterator directly.
type RowsScanner interface {
	RowScanner
	Next() bool
	Err() error
}

var (
	_ RowScanner  = Row{}
	_ RowsScanner = Rows{}
	_ RowsScanner = (*sql.Rows)(nil)
)

// RowScanFunc scans the current row into destinations of the types supplied to
// PrepareScan. It and its scratch storage must not be used concurrently.
type RowScanFunc func(...any) error

// PrepareScan validates the result shape before Next and compiles one reusable
// scan plan. It accepts raw [*sql.Rows] as well as [Rows] and [*Rows], but not
// the single-use [Row] (use [Row.Scan] directly). [Rows] supplies its configured
// conversion policies; other scanners use zero [ScanOptions]. Destination
// pointers may change between calls but their types must stay the same.
func PrepareScan(scanner RowScanner, types ...reflect.Type) (RowScanFunc, error) {
	source, columns, options, _, err := scanSource(scanner)
	if err != nil {
		return nil, err
	}

	plan, err := rowbind.NewPlan(columns, types, options)
	if err != nil {
		return nil, err
	}

	return func(dst ...any) error { return plan.Scan(source, dst) }, nil
}

// Select the underlying scan function before creating a method value. Wrapping
// [Rows.Scan] here would repeat adaptation and retain an unnecessary [Rows] copy.
func scanSource(scanner RowScanner) (func(...any) error, []string, ScanOptions, bool, error) {
	if nilBindingValue(scanner) {
		return nil, nil, ScanOptions{}, false, errors.New("sqlx: nil row scanner")
	}

	var rows Rows
	switch v := scanner.(type) {
	case Rows:
		rows = v

	case *Rows:
		rows = *v

	case Row, *Row:
		return nil, nil, ScanOptions{}, false, errors.New("sqlx: use Row.Scan for a single-use result")

	default:
		columns, err := scanner.Columns()
		_, native := scanner.(*sql.Rows)
		return scanner.Scan, columns, ScanOptions{}, native, err
	}

	columns, err := rows.scanColumns()
	if err != nil {
		return nil, nil, ScanOptions{}, false, err
	}

	return rows.Rows.Scan, columns, rows.config.Scan, true, nil
}

// Internal collection scans have a definite end, so their scratch storage can
// be borrowed. Public PrepareScan functions have caller-controlled lifetimes.
func prepareBindingScan(scanner RowScanner, types ...reflect.Type) (*rowbind.Plan, error) {
	source, columns, options, native, err := scanSource(scanner)
	if err != nil {
		return nil, err
	}
	return rowbind.BorrowPlan(source, columns, types, options, native)
}

// ScanRow adapts positional scalar destinations, including pointer chains.
// For repeated scanning or struct mapping use PrepareScan or [Rows.Scan].
func ScanRow(scan func(...any) error, dst ...any) error {
	return rowbind.ScanScalarRow(scan, dst, ScanOptions{})
}

func scanSingleStruct(rows Rows, dst []any) error {
	columns, err := rows.scanColumns()
	if err != nil {
		return err
	}
	return rowbind.ScanStruct(rows.Rows.Scan, columns, dst, rows.config.Scan)
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
// errors. Use [Rows.Scan] or [PrepareScan] for conversion policies and cached
// plans.
func ScanColumnsToStruct(scan func(...any) error, columns []string, dst any) error {
	return rowbind.ScanColumnsToStruct(scan, columns, dst)
}
