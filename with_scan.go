// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"errors"
	"reflect"

	"github.com/xgfone/go-sqlx/internal/rowbind"
)

// WithScan prepares and lends a type-checked current-row scan function to run.
// It validates the result shape before calling run, even for an empty result.
// *[Rows] supplies its binding labels and [ScanOptions]; other [RowScanner] implementations use
// zero
// [ScanOptions]. Single-use [Row] values are rejected; use [Row.Scan] instead.
//
// Each call covers one result set with fixed labels, options and destination
// types. Destination addresses may change. For *[Rows], [Rows.SetColumns],
// [Rows.SetScanOptions],
// [Rows.SetBindConfig], [Rows.NextResultSet] or [Rows.Close] during run invalidates the scope
// and
// returns an error. For raw scanners, the caller must keep the result set and
// metadata unchanged. Start a new [WithScan] call after changing sets or options.
//
// The scan function borrows its destinations only for each call. Use it only
// synchronously within run; calls after run returns fail. Scratch and source
// references are released on return, error or panic; panics propagate. [WithScan]
// does not advance or close the cursor. The caller owns iteration, [Rows.Err] and [Rows.Close].
func WithScan(scanner RowScanner, types []reflect.Type, run func(scan RowScanFunc) error) error {
	if run == nil {
		return errors.New("sqlx: nil scan callback")
	}

	columns, options, rows, err := scanMetadata(scanner)
	if err != nil {
		return err
	}

	mapping, err := rowbind.Prepare(columns, types, options)
	if err != nil {
		return err
	}

	// The callback and scanner have been checked, and Prepare succeeded.
	// Use prepared entries to avoid repeating scope validation.
	if rows != nil {
		revision := rows.revision
		err := mapping.WithPreparedScan(func(dst ...any) error {
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

	// Keep the scanner in the handle without allocating a source method value.
	// *Rows must keep the guarded source above.
	return mapping.WithPreparedSource(scanner, run)
}

var errScanScopeChanged = errors.New("sqlx: result set or scan configuration changed during WithScan")

// Read the labels and options before creating a scan function, which is only
// needed after preparation succeeds. Only this convenience API infers options.
// Return the identified *Rows so execution can retain its scope guards.
func scanMetadata(scanner RowScanner) ([]string, ScanOptions, *Rows, error) {
	if nilBindingValue(scanner) {
		return nil, ScanOptions{}, nil, errors.New("sqlx: nil row scanner")
	}
	switch rows := scanner.(type) {
	case *Rows:
		columns, err := rows.scanColumns()
		if err != nil {
			return nil, ScanOptions{}, nil, err
		}
		return columns, rows.config.ScanOptions, rows, nil

	case Row, *Row:
		return nil, ScanOptions{}, nil, errors.New("sqlx: use Row.Scan for a single-use result")

	default:
		columns, err := scanner.Columns()
		return columns, ScanOptions{}, nil, err
	}
}
