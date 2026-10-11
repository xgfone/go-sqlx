// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package rowbind

import (
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"time"
)

// Cursor scans raw current-row destinations positionally, honoring [sql.Scanner].
// [Cursor.Scan] must not apply another [github.com/xgfone/go-sqlx] mapping or
// conversion layer. It borrows dst for the call; retaining the slice or a
// subslice requires a clone. Adapter scanners must be consumed synchronously
// even if the slice is cloned. Columns and policies come from [Prepare].
// The caller owns iteration and cursor closing.
type Cursor interface {
	Scan(...any) error
	Next() bool
	Err() error
}

// RowScanFunc scans one row into the supplied destinations. The source function
// determines whether scanning is positional or applies struct mapping.
// It borrows the destination slice only for the call; retaining the slice or a
// subslice requires a copy. Temporary scanner adapters must be used synchronously.
// Functions lent by a [Mapping] are valid only within their callbacks.
type RowScanFunc func(...any) error

// Mapping is immutable preparation for one ordered result shape and destination
// signature. Copies may be shared concurrently; each execution owns its scratch.
// Its zero value is not prepared.
type Mapping struct {
	// Common time policies stay inline; other conversions use an optional snapshot.
	location *time.Location
	options  *ScanOptions
	layout   *structScanLayout

	// Most bindings scan one element or a key/value pair. Keep those signatures
	// inline; wider positional mappings own a single combined descriptor slice.
	types [2]reflect.Type
	extra []scalarDestination
	flags mappingFlags
}

type scalarDestination struct {
	typeOf  reflect.Type
	wrapped bool
}

type mappingFlags uint8

// Paired flags stay adjacent because preparation indexes them with << i.
const (
	mappingPrepared mappingFlags = 1 << iota
	mappingReusableFirst
	mappingReusableSecond
	mappingWrappedFirst
	mappingWrappedSecond
	mappingAllowZeroDate
)

// Prepare validates a mapping without borrowing scratch or touching a cursor or
// destination. It snapshots option values, columns and types, sharing immutable
// TimeLayouts.
func Prepare(columns []string, types []reflect.Type, options ScanOptions) (Mapping, error) {
	if err := options.validate(); err != nil {
		return Mapping{}, err
	}

	m := Mapping{flags: mappingPrepared, location: options.Location}
	if options.AllowZeroDate {
		m.flags |= mappingAllowZeroDate
	}

	t, err := structScanType(types)
	if err != nil {
		return Mapping{}, err
	}

	if t != nil {
		m.layout, err = prepareStructLayout(columns, t, options)
		if err != nil {
			return Mapping{}, err
		}

		m.types[0] = types[0]
		if m.layout.reusable {
			m.flags |= mappingReusableFirst
		}

		m.captureOptions(options)
		return m, nil
	}

	if len(columns) != len(types) {
		return Mapping{}, fmt.Errorf("sqlx: %d result columns for %d destinations",
			len(columns), len(types))
	}
	if len(types) > len(m.types) {
		m.extra = make([]scalarDestination, len(types))
	}

	for i, t := range types {
		wrapped, err := scalarDestinationWrapper(t, i)
		if err != nil {
			return Mapping{}, err
		}

		if i < 2 && (wrapped || reusableScannerType(t)) {
			m.flags |= mappingReusableFirst << i
		}

		if m.extra != nil {
			m.extra[i] = scalarDestination{t, wrapped}
		} else {
			m.types[i] = t
			if wrapped {
				m.flags |= mappingWrappedFirst << i
			}
		}
	}

	m.captureOptions(options)
	return m, nil
}

// Only successful mappings need a conversion snapshot. Layouts already capture
// mapping policies, and TimeLayouts shares its immutable backing array.
func (m *Mapping) captureOptions(options ScanOptions) {
	if len(options.TimeLayouts) != 0 || options.DurationUnit != 0 || options.Nulls != 0 {
		captured := options
		m.options = &captured
	}
}

func (m Mapping) init(p *scanPlan) {
	options := ScanOptions{
		Location:      m.location,
		AllowZeroDate: m.flags&mappingAllowZeroDate != 0,
	}
	if m.options != nil {
		options = *m.options
	}
	if m.layout != nil {
		initStructLayout(p, m.layout, m.types[0], options)
		return
	}

	types := m.types[:0]
	if m.extra != nil {
		// Own the grown scratch before initialization so initStorage reuses it.
		// Do not store types itself: the inline branch borrows m.types.
		p.types = reuseScanStorage(p.types, len(m.extra))
		types = p.types
		for i, dst := range m.extra {
			types[i] = dst.typeOf
		}
	} else {
		for _, t := range m.types {
			if t == nil {
				break
			}
			types = append(types, t)
		}
	}

	p.initStorage(len(types), types, options)
	p.scanners = reuseScanStorage(p.scanners, len(types))
	p.wrapped = reuseScanStorage(p.wrapped, len(types))
	for i := range p.scanners {
		p.scanners[i] = options.scanner(nil)
		if m.extra != nil {
			p.wrapped[i] = m.extra[i].wrapped
		} else {
			p.wrapped[i] = m.flags&(mappingWrappedFirst<<i) != 0
		}
	}
}

// WithScan lends a current-row scan function to one synchronous operation. The
// callback must not retain it. Destination types must match the prepared types.
// Scratch is cleared and returned on success, error and panic. reusable reports
// whether map temporaries may safely be reused with this mapping and cursor.
// Application [sql.Scanner] implementations receive borrowed inputs synchronously;
// they must copy bytes they retain. Nullable-parent layouts own their deferred
// input storage.
func (m Mapping) WithScan(cursor Cursor, run func(scan RowScanFunc, reusable Reuse) error) error {
	if m.flags&mappingPrepared == 0 || nilBindingValue(cursor) || run == nil {
		return errors.New("sqlx: expected a prepared mapping, cursor and scan operation")
	}

	p := scanPlanPool.Get().(*scanPlan)
	defer releasePlan(p)
	m.init(p)

	if rows, raw := cursor.(*sql.Rows); raw {
		reusable := Reuse{
			m.flags&mappingReusableFirst != 0,
			m.flags&mappingReusableSecond != 0,
		}

		// Reuse the concrete source already identified for the reuse policy.
		// Its ScanSource conversion is static and the callback captures one pointer.
		return run(func(dst ...any) error {
			return p.scanSourceValues(rows, dst)
		}, reusable)
	}

	// Narrow the custom cursor once per operation, outside the row callback.
	return run(p.sourceScan(ScanSource(cursor)), Reuse{})
}

// withSourceScan requires a prepared mapping, non-nil source (including its
// concrete value) and callback. Callers have already excluded raw *sql.Rows;
// custom cursors never receive destination reuse permissions.
func (m *Mapping) withSourceScan(source ScanSource, run func(RowScanFunc, Reuse) error) error {
	p := scanPlanPool.Get().(*scanPlan)
	defer releasePlan(p)

	m.init(p)
	return run(p.sourceScan(source), Reuse{})
}

// Share callback construction without adding another scope call to WithScan.
func (p *scanPlan) sourceScan(source ScanSource) RowScanFunc {
	return func(dst ...any) error {
		return p.scanSourceValues(source, dst)
	}
}
