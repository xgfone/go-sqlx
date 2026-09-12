// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package rowbind

import (
	"database/sql"
	"errors"
	"fmt"
	"reflect"
)

// Cursor scans raw current-row destinations positionally, honoring sql.Scanner.
// Scan must not apply another sqlx mapping or conversion layer. It borrows dst
// for the call; retaining the slice or a subslice requires a clone. Adapter
// scanners must be consumed synchronously even if the slice is cloned. Columns
// and policies come from Prepare. The caller owns iteration and cursor closing.
type Cursor interface {
	Scan(...any) error
	Next() bool
	Err() error
}

// Mapping is immutable preparation for one ordered result shape and destination
// signature. Copies may be shared concurrently; each execution owns its scratch.
// Its zero value is not prepared.
type Mapping struct {
	options *ScanOptions
	layout  *structScanLayout

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
)

// Prepare validates a mapping without borrowing scratch or touching a cursor or
// destination. It takes snapshots of mutable input slices.
func Prepare(columns []string, types []reflect.Type, options ScanOptions) (Mapping, error) {
	if err := options.validate(); err != nil {
		return Mapping{}, err
	}

	m := Mapping{flags: mappingPrepared}
	// Mapping policies are captured by the layout. Only nonzero conversion
	// options need a private snapshot, including caller-owned time layouts.
	if options.Location != nil || len(options.TimeLayouts) != 0 || options.DurationUnit != 0 ||
		options.AllowZeroDate || options.Nulls != 0 {
		owned := options.clone()
		m.options = &owned
	}

	if len(types) == 1 && types[0] != nil && !IsScalarDestination(types[0]) {
		t := types[0]
		if t.Kind() != reflect.Pointer {
			return Mapping{}, errors.New("sqlx: expected pointer destination")
		}

		var err error
		if t, err = indirectType(t); err != nil {
			return Mapping{}, err
		}

		if t.Kind() == reflect.Struct {
			m.layout, err = prepareStructLayout(columns, t, options)
			if err != nil {
				return Mapping{}, err
			}

			m.types[0] = types[0]
			if m.layout.reusable {
				m.flags |= mappingReusableFirst
			}

			return m, nil
		}
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

	return m, nil
}

func (m Mapping) init(p *scanPlan) {
	var options ScanOptions
	if m.options != nil {
		options = *m.options
	}
	if m.layout != nil {
		initStructLayout(p, m.layout, m.types[0], options)
		return
	}

	types := m.types[:0]
	if m.extra != nil {
		types = reuseScanStorage(p.types, len(m.extra))
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

// Reuse reports independent temporary-destination safety for the first two
// targets (MapPairs), or the single target of MapIndex/MapSet. Unknown raw
// cursors never permit reuse even when destination types themselves are safe.
type Reuse [2]bool

// WithScan lends a current-row scan function to one synchronous operation. The
// callback must not retain it. Destination types must match the prepared types.
// Scratch is cleared and returned on success, error and panic. reusable reports
// whether map temporaries may safely be reused with this mapping and cursor.
func (m Mapping) WithScan(cursor Cursor, run func(scan func(...any) error, reusable Reuse) error) error {
	if m.flags&mappingPrepared == 0 || nilBindingValue(cursor) || run == nil {
		return errors.New("sqlx: expected a prepared mapping, cursor and scan operation")
	}

	p := scanPlanPool.Get().(*scanPlan)
	defer releasePlan(p)
	m.init(p)

	var reusable Reuse
	if _, raw := cursor.(*sql.Rows); raw {
		reusable = Reuse{
			m.flags&mappingReusableFirst != 0,
			m.flags&mappingReusableSecond != 0,
		}
	}

	source := cursor.Scan
	return run(func(dst ...any) error {
		return p.scanValues(source, dst)
	}, reusable)
}

// Scanner borrows private scratch until Close. Each scanner validates destination
// types and must not be copied or used concurrently. Closing it releases only
// scanning resources; ownership of the source stays with the caller.
func (m Mapping) Scanner(source func(...any) error) (*Scanner, error) {
	if m.flags&mappingPrepared == 0 || source == nil {
		return nil, errors.New("sqlx: expected a prepared mapping and scan function")
	}

	p := scanPlanPool.Get().(*scanPlan)
	m.init(p)
	return &Scanner{plan: p, source: source}, nil
}

// Scanner owns a prepared scan's mutable storage until Close. Its zero value
// is closed. A closed scanner must never regain access to a recycled plan.
type Scanner struct {
	plan   *scanPlan
	source func(...any) error
}

func (s *Scanner) Scan(dst ...any) error {
	if s == nil || s.plan == nil {
		return errors.New("sqlx: prepared scanner is closed")
	}
	return s.plan.Scan(s.source, dst)
}

// Close releases scratch and the source reference. Repeated calls are harmless.
// The Scanner handle itself is never pooled, so stale aliases remain closed.
func (s *Scanner) Close() error {
	if s != nil {
		p := s.plan
		s.plan, s.source = nil, nil
		if p != nil {
			releasePlan(p)
		}
	}
	return nil
}

// ScanState reuses private scratch for manual scans of one result set. It must
// not be copied or used concurrently. Reset it when columns or options change,
// when advancing result sets, and when closing the result. Scans must consume
// their arguments synchronously. No cursor or destination is retained.
type ScanState struct{ plan *scanPlan }

// Reset releases cached preparation and scratch. It is safe to call repeatedly.
func (s *ScanState) Reset() {
	if p := s.plan; p != nil {
		s.plan = nil
		releasePlan(p)
	}
}

// Scan reuses preparation while destination types match. Columns and options
// must remain immutable until Reset. Each call clears destination references,
// including when conversion fails or a custom scanner panics.
func (s *ScanState) Scan(source func(...any) error, columns []string, dst []any, options ScanOptions) error {
	if source == nil {
		return errors.New("sqlx: nil scan function")
	}
	if s.plan != nil && s.plan.matches(dst) {
		return s.plan.scanValues(source, dst)
	}

	s.Reset()
	p := scanPlanPool.Get().(*scanPlan)
	types := reuseScanStorage(p.types, len(dst))
	for i, d := range dst {
		types[i] = reflect.TypeOf(d)
	}

	if _, err := initRowScanPlan(p, columns, types, options); err != nil {
		releasePlan(p)
		return err
	}

	s.plan = p
	return p.scanValues(source, dst)
}
