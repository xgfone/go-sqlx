// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"errors"
	"reflect"

	"github.com/xgfone/go-sqlx/internal/rowbind"
)

// Visit scans the current result set and calls yield once per successful row.
// Returning false stops normally; returning an error stops with a BindError for
// that row. Delivered values may be retained and are not overwritten by later
// rows. Custom Scanner implementations still own their buffer-copying duties.
// Callbacks must not advance, scan, close, reconfigure or concurrently use r.
//
// Visit closes r on completion, an early stop, an error or a yield panic.
// Application Scanners must not panic: sqlx cannot guarantee cursor cleanup
// after a panic inside the underlying Scan. Callback side effects are not rolled
// back. A close error is returned, or joined with an earlier error. Iteration
// errors report the next row being requested. No subsequent result set is visited
// automatically.
//
// Like Scan, Visit uses the result's labels and ScanOptions directly, ignoring
// collection-level RowsBinder registrations, Capacity and DuplicateKeys.
func (r *Rows) Visit[T any](yield func(T) (continueReading bool, err error)) (err error) {
	defer r.closeConsumedRows(&err)

	if yield == nil {
		return errors.New("sqlx: nil row visitor")
	}

	columns, err := r.scanColumns()
	if err != nil {
		return err
	}

	mapping, err := rowbind.Prepare(columns, []reflect.Type{reflect.TypeFor[*T]()}, r.config.ScanOptions)
	if err != nil {
		return err
	}

	return mapping.WithVisitScan(r.rows, func(scan func(...any) error, reusable rowbind.Reuse) error {
		return visitRows(r.rows, scan, yield, reusable[0])
	})
}

func visitRows[T any](
	cursor RowCursor,
	scan func(...any) error,
	yield func(T) (bool, error),
	reusable bool,
) error {
	args := [1]any{}
	defer clear(args[:])

	var shared *T
	var zero T
	row := 0
	for cursor.Next() {
		row++
		value := shared
		if value == nil {
			value = new(T)
			if reusable {
				shared = value
			}
		}

		*value = zero
		args[0] = value
		if err := scan(args[:]...); err != nil {
			return &BindError{row, err}
		}

		more, err := yield(*value)
		if err != nil {
			return &BindError{row, err}
		}

		if !more {
			break
		}
	}

	if err := cursor.Err(); err != nil {
		return &BindError{row + 1, err}
	}
	return nil
}

// Partial-result operations retain both the primary failure and a close error.
// Atomic binding keeps its existing error precedence in Rows.bind.
func (r *Rows) closeConsumedRows(err *error) {
	if closeErr := r.Close(); closeErr != nil {
		if *err == nil {
			*err = closeErr
		} else if !errors.Is(*err, closeErr) {
			*err = errors.Join(*err, closeErr)
		}
	}
}
