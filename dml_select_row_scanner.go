// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"database/sql"
	"errors"
	"reflect"
	"slices"

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

// PreparedScanner scans the current row using a prepared destination signature.
// Scan borrows dst for the call; retaining the slice or a subslice requires a
// clone. This does not change the scanner's own lifetime, which ends at Close.
// Call Close when finished, including after scan errors or panics, to release
// its scratch and source references. Close does not close or advance the cursor;
// the caller owns that separately. Close is idempotent; Scan after Close fails.
// Scan and Close must not be called concurrently.
type PreparedScanner interface {
	Scan(...any) error
	Close() error
}

// PrepareScan validates the result shape before Next and prepares a reusable
// scanner. *Rows supplies its current conversion options and labels; raw
// *sql.Rows and other scanners use zero ScanOptions. Row is single-use and is
// rejected. Destination addresses may change but their types must stay fixed.
// Scanners prepared from *Rows follow its SetColumns, SetScanOptions,
// SetBindConfig and NextResultSet calls. For other scanners, prepare again after
// changing sets. Close the prepared scanner separately from its cursor.
// The scanner and cursor must not be used concurrently.
func PrepareScan(scanner RowScanner, types ...reflect.Type) (PreparedScanner, error) {
	source, columns, options, err := scanSource(scanner)
	if err != nil {
		return nil, err
	}

	mapping, err := rowbind.Prepare(columns, types, options)
	if err != nil {
		return nil, err
	}

	scan, err := mapping.Scanner(source)
	if err != nil {
		return nil, err
	}

	rows, ok := scanner.(*Rows)
	if !ok {
		return scan, nil
	}

	return &preparedRowsScanner{
		rows:     rows,
		scan:     scan,
		types:    slices.Clone(types),
		revision: rows.revision,
	}, nil
}

type preparedRowsScanner struct {
	rows     *Rows
	scan     *rowbind.Scanner
	types    []reflect.Type
	revision uint64
}

func (s *preparedRowsScanner) Scan(dst ...any) error {
	if s.scan == nil {
		return errors.New("sqlx: prepared scanner is closed")
	}
	if err := s.rows.Err(); err != nil {
		return err
	}

	if s.revision != s.rows.revision {
		columns, err := s.rows.scanColumns()
		if err != nil {
			return err
		}

		mapping, err := rowbind.Prepare(columns, s.types, s.rows.config.Scan)
		if err != nil {
			return err
		}

		next, err := mapping.Scanner(s.rows.rows.Scan)
		if err != nil {
			return err
		}

		_ = s.scan.Close()
		s.scan, s.revision = next, s.rows.revision
	}

	return s.scan.Scan(dst...)
}

func (s *preparedRowsScanner) Close() error {
	if s.scan != nil {
		_ = s.scan.Close()
		s.scan = nil
	}
	s.rows, s.types = nil, nil
	return nil
}

// Select the raw function so a prepared scan does not repeat Rows.Scan's
// mapping and adaptation. Only this public convenience API infers configuration.
func scanSource(scanner RowScanner) (func(...any) error, []string, ScanOptions, error) {
	if nilBindingValue(scanner) {
		return nil, nil, ScanOptions{}, errors.New("sqlx: nil row scanner")
	}
	switch rows := scanner.(type) {
	case *Rows:
		columns, err := rows.scanColumns()
		if err != nil {
			return nil, nil, ScanOptions{}, err
		}
		return rows.rows.Scan, columns, rows.config.Scan, nil

	case Row, *Row:
		return nil, nil, ScanOptions{}, errors.New("sqlx: use Row.Scan for a single-use result")

	default:
		columns, err := scanner.Columns()
		return scanner.Scan, columns, ScanOptions{}, err
	}
}

// ScanRow adapts positional scalar destinations, including pointer chains.
// For repeated scanning or struct mapping use PrepareScan or [Rows.Scan].
// The callback borrows its destination slice and must clone it before retaining
// it or a subslice. Temporary scanner adapters are valid only during the call.
func ScanRow(scan func(...any) error, dst ...any) error {
	return rowbind.ScanScalarRow(scan, dst, ScanOptions{})
}

func scanSingleStruct(rows *Rows, dst []any) error {
	columns, err := rows.scanColumns()
	if err != nil {
		return err
	}
	return rowbind.ScanStruct(rows.rows.Scan, columns, dst, rows.config.Scan)
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
// plans. The callback borrows its destination slice only for the call; it must
// use slices.Clone(dst) or an equivalent copy before retaining it or a subslice.
// The cloned entries still point to the caller's fields. Internal scratch is
// released automatically on success, error or panic.
func ScanColumnsToStruct(scan func(...any) error, columns []string, dst any) error {
	return rowbind.ScanColumnsToStruct(scan, columns, dst)
}
