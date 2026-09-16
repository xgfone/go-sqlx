// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"context"
	"database/sql"
	"errors"
)

var errRawVisitActive = errors.New("sqlx: Rows is in a raw-byte visitor callback")

// The internal marker belongs to one [Rows]. Returning the public error and using
// it to construct another [Rows] must not prevent that other cursor from closing.
type rawVisitGuard struct{ owner *Rows }

func (*rawVisitGuard) Error() string {
	return errRawVisitActive.Error()
}

func (r *Rows) inRawVisit() bool {
	guard, ok := r.err.(*rawVisitGuard)
	return ok && guard.owner == r
}

// VisitRawBytes reads all columns as [sql.RawBytes], in result order. The row slice
// and its bytes are read-only and valid only until yield returns. Clone any
// bytes to be retained; copying the row's slice headers alone is insufficient.
// SQL NULL becomes nil. Empty values may also become nil depending on the driver
// and [database/sql] conversion, so nil alone does not establish SQL NULL. Select
// a separate IS NULL column when that distinction is required.
//
// Conversion follows [sql.RawBytes], without sqlx [ScanOptions] or struct
// mapping. [RowsBinder], [BindConfig.Capacity] and duplicate-key policies are not applied.
// Column label overrides are validated but do not change positional values.
// Pass the query's context: it is checked between reads/callbacks and during
// finalization. It does not replace the context of an already running query.
//
// Returning false stops normally; callback/scan errors include the current row
// number. Iteration errors refer to the next row. The current result is closed
// on completion, early stop, error or panic, and close errors are preserved.
//
// Do not use r or its underlying [sql.Rows] from yield. Reentrant cursor methods
// return an error/false, and Set methods do nothing. Concurrent use is unsupported.
// Cancellation cannot invalidate the bytes during yield, but cursor closing may
// wait for yield to return. The callback must not wait for that close to finish.
func (r *Rows) VisitRawBytes(ctx context.Context, yield func([]sql.RawBytes) (bool, error)) (err error) {
	visitor := rawRowsVisitor{rows: r}
	defer visitor.finish(ctx, &err)

	if ctx == nil {
		return errors.New("sqlx: nil raw-byte visitor context")
	}
	if yield == nil {
		return errors.New("sqlx: nil raw-byte visitor")
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	columns, err := r.scanColumns()
	if err != nil {
		return err
	}

	visitor.values = make([]sql.RawBytes, len(columns))
	visitor.args = make([]any, len(columns))
	visitor.guard = &rawVisitGuard{owner: r}
	for i := range visitor.args {
		visitor.args[i] = &visitor.values[i]
	}
	return visitor.run(ctx, yield)
}

type rawRowsVisitor struct {
	rows    *Rows
	values  []sql.RawBytes
	args    []any
	guard   *rawVisitGuard
	row     int
	started bool
}

func (v *rawRowsVisitor) finish(ctx context.Context, err *error) {
	// 1. Clear references retained by the visitor's scan buffers.
	clear(v.values)
	clear(v.args)

	// 2. Close the result set and preserve any close error.
	v.rows.closeConsumedRows(err)

	// 3. Close releases the lock retained by RawBytes before Err takes another
	// read lock. Calling Err earlier could deadlock behind a cancellation close.
	if v.started {
		for _, e := range [2]error{v.rows.rows.Err(), ctx.Err()} {
			if e == nil || errors.Is(*err, e) {
				continue
			}

			positioned := &BindError{v.row + 1, e}
			if *err == nil {
				*err = positioned
			} else {
				*err = errors.Join(*err, positioned)
			}
		}
	}
}

func (v *rawRowsVisitor) run(ctx context.Context, yield func([]sql.RawBytes) (bool, error)) error {
	v.started = true
	for {
		if err := ctx.Err(); err != nil {
			return &BindError{v.row + 1, err}
		}
		if !v.rows.rows.Next() {
			break
		}

		v.row++
		if err := v.rows.rows.Scan(v.args...); err != nil {
			return &BindError{v.row, err}
		}

		more, err := v.rows.yieldRawBytes(v.values, yield, v.guard)
		if err != nil {
			return &BindError{v.row, err}
		}

		if !more {
			break
		}
	}
	return nil
}

func (r *Rows) yieldRawBytes(
	values []sql.RawBytes,
	yield func([]sql.RawBytes) (bool, error),
	guard *rawVisitGuard,
) (bool, error) {
	r.err = guard
	defer func() { r.err = nil }()
	return yield(values)
}
