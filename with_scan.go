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
