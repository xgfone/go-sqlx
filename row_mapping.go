// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"errors"
	"reflect"

	"github.com/xgfone/go-sqlx/internal/rowbind"
)

// RowMapping is immutable preparation for one ordered result shape. Copies can
// be shared concurrently; scanners created from it own independent state.
type RowMapping struct{ mapping rowbind.Mapping }

// PrepareMapping lets a custom binder prepare conversion and struct mapping
// before it receives a cursor. It snapshots mutable option slices and does not
// allocate execution scratch or change any destination.
func (o BindOptions) PrepareMapping(types ...reflect.Type) (RowMapping, error) {
	mapping, err := rowbind.Prepare(o.Columns, types, o.ScanOptions)
	return RowMapping{mapping: mapping}, err
}

// WithScan lends a type-checked current-row scan function to run. Each call
// borrows independent scratch, released when run returns or panics. Panics
// propagate. The scan function is valid only synchronously within run; later
// calls fail. Destination addresses may change, but their types must match the
// prepared signature. The callback must keep the cursor on the same result set.
//
// The caller owns iteration, Err and cursor closing. The cursor must obey
// RowCursor's raw-scan contract; *Rows is rejected to avoid a second mapping and
// conversion layer. Use the package-level WithScan for *Rows instead.
func (m RowMapping) WithScan(cursor RowCursor, run func(scan RowScanFunc) error) error {
	if nilBindingValue(cursor) {
		return errors.New("sqlx: nil row cursor")
	}

	if _, ok := cursor.(*Rows); ok {
		return errors.New("sqlx: RowMapping.WithScan requires a raw cursor; use WithScan for *Rows")
	}

	return m.mapping.WithCheckedScan(cursor.Scan, run)
}
