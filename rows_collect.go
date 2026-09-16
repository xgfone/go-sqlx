// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"reflect"
	"slices"

	"github.com/xgfone/go-sqlx/internal/rowbind"
)

// Collect consumes and closes the current result set, returning a typed slice.
// It inherits [Rows]' labels, [ScanOptions] and capacity hint. Explicit binders and
// exact registry registrations remain authoritative, including on failure;
// an unregistered slice uses the native typed slice binder.
//
// Built-in binding scans directly into staged elements and returns a non-nil
// empty slice on an empty result. Any error returns nil, including close errors.
func (r *Rows) Collect[T any]() ([]T, error) {
	var values []T
	err := r.bind(&values, BindReplace, typedSliceRowsBinder[[]T, T]{})
	if err != nil {
		return nil, err
	}
	return values, nil
}

// CollectInto replaces storage's contents, reusing its backing array, and closes
// the current result set. The caller grants exclusive write access through
// cap(storage), including elements beyond its length. Other aliases may change.
// Always retain the returned slice: it may grow and preserves named slice types.
//
// Errors return the successfully scanned prefix, including on a close error.
// The failed slot and unused capacity are cleared, even on preparation errors or
// panics. Empty results retain the supplied buffer without reserving more space;
// nil storage remains nil. Existing capacity is used before consulting the
// result's capacity hint for growth. This hint never limits the number of rows.
//
// Like [Rows.Scan], this method uses the result's labels and [ScanOptions] directly;
// collection-level [RowsBinder] registrations are not applied. [Rows.Bind] and [Rows.Collect]
// provide independent storage and atomic publication instead. Custom [sql.Scanner]
// side effects and panics follow [Rows.Scan]'s contract.
func (r *Rows) CollectInto[S ~[]T, T any](storage S) (result S, err error) {
	defer r.closeConsumedRows(&err)

	collector := sliceCollector[S, T]{values: storage[:0]}
	defer collector.clearUnused()

	options, err := r.bindOptions(BindReplace)
	if err != nil {
		return collector.values, err
	}

	mapping, err := rowbind.Prepare(options.Columns, []reflect.Type{reflect.TypeFor[*T]()}, options.ScanOptions)
	if err != nil {
		return collector.values, err
	}

	collector.capacity = options.capacity()
	err = mapping.WithScan(r.rows, func(scan RowScanFunc, _ rowbind.Reuse) error {
		return collector.scanRows(r.rows, scan)
	})

	return collector.values, err
}

// sliceCollector owns the writable capacity, including an uncommitted scan slot.
// Its length advances only after a complete row has been scanned successfully.
type sliceCollector[S ~[]T, T any] struct {
	values   S
	args     [1]any
	capacity int
}

func (c *sliceCollector[S, T]) clearUnused() {
	clear(c.args[:])
	clear(c.values[len(c.values):cap(c.values)])
}

func (c *sliceCollector[S, T]) scanRows(cursor RowCursor, scan RowScanFunc) error {
	var zero T
	for cursor.Next() {
		n := len(c.values)
		if n == cap(c.values) {
			if n == 0 {
				c.values = make(S, 0, c.capacity)
			} else {
				c.values = slices.Grow(c.values, max(c.capacity-n, 1))
			}
		}

		pending := c.values[:n+1]
		pending[n] = zero
		c.args[0] = &pending[n]
		if err := scan(c.args[:]...); err != nil {
			return &BindError{n + 1, err}
		}

		c.values = pending
	}

	if err := cursor.Err(); err != nil {
		return &BindError{len(c.values) + 1, err}
	}
	return nil
}
