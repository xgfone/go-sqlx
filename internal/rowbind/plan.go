// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package rowbind

import (
	"errors"
	"fmt"
	"reflect"
	"slices"
	"sync"
)

// ScanCurrent uses the source passed to BorrowPlan. Destination types must match
// the prepared types; collection binders establish this before iteration.
func (p *Plan) ScanCurrent(dst ...any) error { return p.ScanValues(p.source, dst) }

// Reusing map temporaries is safe only with database/sql and conversions that
// cannot retain a pointer to the destination. Custom scanners keep fresh values.
func (p *Plan) ReusableMapValues() bool {
	if !p.nativeSource {
		return false
	}
	if p.layout != nil {
		return p.layout.reusable
	}

	for _, wrapped := range p.wrapped {
		if !wrapped {
			return false
		}
	}

	return true
}

// ScanScalarRow validates and adapts positional scalar destinations once.
func ScanScalarRow(scan func(...any) error, dst []any, options ScanOptions) error {
	if scan == nil {
		return errors.New("sqlx: nil scan function")
	}
	if err := options.validate(); err != nil {
		return err
	}

	var adapted []any
	for i, v := range dst {
		if nilBindingValue(v) {
			return fmt.Errorf("sqlx: nil destination at column %d", i)
		}

		t := reflect.TypeOf(v)
		if t.Implements(_scannertype) {
			continue
		}

		if t.Kind() != reflect.Pointer || !IsScalarDestination(t) {
			return fmt.Errorf("sqlx: unsupported scalar destination %v at column %d", t, i)
		}

		if adapted == nil {
			adapted = slices.Clone(dst)
		}
		adapted[i] = options.scanner(v)
	}

	if adapted != nil {
		dst = adapted
	}
	return scan(dst...)
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

// Plan is mutable per-result scan state. It must not be copied or used concurrently.
// Fields, layouts and setters remain private so callers depend only on operations.
type Plan struct {
	options       ScanOptions
	nativeSource  bool
	source        func(...any) error
	layout        *structScanLayout
	structDstType reflect.Type
	types         []reflect.Type
	scanners      []GeneralScanner
	fieldScanners []fieldScanner
	values        []any
	wrapped       []bool
	captured      []captureScanner
	skip          []bool
}

type nullStructGroup struct {
	path    []int
	columns []int
}

type captureScanner struct{ value any }

func (s *captureScanner) Scan(value any) error {
	s.value = value
	return nil
}

type discardScanner struct{}

func (discardScanner) Scan(any) error { return nil }

// NewPlan creates caller-owned storage for prepared/manual scans. Conversion
// option slices must remain immutable for the lifetime of the plan.
func NewPlan(columns []string, types []reflect.Type, options ScanOptions) (*Plan, error) {
	return initRowScanPlan(&Plan{}, columns, types, options)
}

func initRowScanPlan(p *Plan, columns []string, types []reflect.Type, options ScanOptions) (*Plan, error) {
	if err := options.validate(); err != nil {
		return nil, err
	}

	if len(types) == 1 && types[0] != nil && !IsScalarDestination(types[0]) {
		t := types[0]
		if t.Kind() != reflect.Pointer {
			return nil, errors.New("sqlx: expected pointer destination")
		}

		var err error
		if t, err = indirectType(t); err != nil {
			return nil, err
		}

		if t.Kind() == reflect.Struct {
			return initStructScanPlan(p, columns, types[0], t, options)
		}
	}

	return initScalarScanPlan(p, columns, types, options)
}

func initScalarScanPlan(p *Plan, columns []string, types []reflect.Type, options ScanOptions) (*Plan, error) {
	if err := options.validate(); err != nil {
		return nil, err
	}

	if len(columns) != len(types) {
		return nil, fmt.Errorf("sqlx: %d result columns for %d destinations", len(columns), len(types))
	}

	p.initStorage(len(columns), types, options)
	p.scanners = reuseScanStorage(p.scanners, len(columns))
	for i := range p.scanners {
		p.scanners[i] = options.scanner(nil)
	}

	p.wrapped = reuseScanStorage(p.wrapped, len(types))
	for i, t := range types {
		custom := t != nil && t.Implements(_scannertype) && t.Kind() != reflect.Interface
		if t == nil || (!custom && (t.Kind() != reflect.Pointer || !IsScalarDestination(t))) {
			return nil, fmt.Errorf("sqlx: unsupported scalar destination %v at column %d", t, i)
		}
		p.wrapped[i] = !custom
	}

	return p, nil
}

func reuseScanStorage[T any](values []T, count int) []T {
	if cap(values) < count {
		return make([]T, count)
	}
	return values[:count]
}

func (p *Plan) initStorage(count int, types []reflect.Type, options ScanOptions) {
	p.types = reuseScanStorage(p.types, len(types))
	copy(p.types, types)

	p.values = reuseScanStorage(p.values, count)
	p.scanners = p.scanners[:0]
	p.fieldScanners = p.fieldScanners[:0]
	p.options = options

	p.layout = nil
	p.structDstType = nil
}

// Only scans with a known synchronous lifetime borrow this scratch pool.
// Large plans are discarded, and no result, configuration or destination is
// retained. sync.Pool may drop storage at any GC; correctness never relies on it.
var scanPlanPool = sync.Pool{New: func() any { return &Plan{} }}

// Release clears all references and returns a borrowed plan to the scratch pool.
// The caller must not use the plan after release.
func Release(p *Plan) {
	p.releaseDestinations()
	clear(p.values)
	clear(p.types)
	clear(p.scanners)
	clear(p.fieldScanners)
	clear(p.captured)
	p.options = ScanOptions{}
	p.layout = nil
	p.source = nil
	p.structDstType = nil
	p.nativeSource = false
	if cap(p.types) <= 64 && cap(p.values) <= 64 && cap(p.scanners) <= 64 && cap(p.fieldScanners) <= 64 {
		scanPlanPool.Put(p)
	}
}

// ScanStruct borrows scratch storage for one synchronous struct scan.
// dst must contain exactly one struct pointer; scan must not retain its arguments.
func ScanStruct(scan func(...any) error, columns []string, dst []any, options ScanOptions) error {
	if len(dst) != 1 || dst[0] == nil || scan == nil {
		return errors.New("sqlx: expected one struct destination and a scan function")
	}

	p := scanPlanPool.Get().(*Plan)
	defer Release(p)

	dstType := reflect.TypeOf(dst[0])
	if dstType.Kind() != reflect.Pointer {
		return errors.New("sqlx: expected pointer destination")
	}

	t, err := indirectType(dstType)
	if err != nil {
		return err
	}

	if t.Kind() != reflect.Struct {
		return fmt.Errorf("sqlx: unsupported destination %v", dstType)
	}

	_, err = initStructScanPlan(p, columns, dstType, t, options)
	if err != nil {
		return err
	}

	return p.Scan(scan, dst)
}

// Matches reports whether the destination count and types match this plan.
func (p *Plan) Matches(dst []any) bool {
	if p.layout != nil {
		return len(dst) == 1 && reflect.TypeOf(dst[0]) == p.structDstType
	}
	if len(dst) != len(p.types) {
		return false
	}

	for i, v := range dst {
		if reflect.TypeOf(v) != p.types[i] {
			return false
		}
	}

	return true
}

// Scan checks destination types before scanning. The scan function must be non-nil.
func (p *Plan) Scan(scan func(...any) error, dst []any) error {
	if !p.Matches(dst) {
		return errors.New("sqlx: prepared scan destination types changed")
	}
	return p.ScanValues(scan, dst)
}

// ScanValues scans destinations whose count and types are already validated.
// The scan function must be non-nil. Use Scan when types can change between rows.
func (p *Plan) ScanValues(scan func(...any) error, dst []any) error {
	if p.layout != nil {
		defer p.releaseDestinations()
		return p.scanStruct(scan, dst[0])
	}
	defer p.releaseScalarDestinations()

	for i, v := range dst {
		if nilBindingValue(v) {
			return fmt.Errorf("sqlx: nil destination at column %d", i)
		}

		if p.wrapped[i] {
			p.scanners[i].Value = v
			p.values[i] = &p.scanners[i]
		} else {
			p.values[i] = v
		}
	}

	return scan(p.values...)
}

// Validated positional plans have equally sized vectors. Keep this cleanup
// independent of struct layouts and their larger reflection scratch state.
func (p *Plan) releaseScalarDestinations() {
	for i := range p.values {
		p.values[i] = nil
		p.scanners[i].Value = nil
	}
}

func (p *Plan) releaseDestinations() {
	if p.layout != nil && len(p.layout.groups) == 0 {
		// Built-in wrappers only point into this plan. Keep that stable argument
		// vector across rows; clear the actual destinations below. Custom fields
		// contain caller addresses directly and must release them after each scan.
		if !p.layout.reusable {
			for i, field := range p.layout.fields {
				if field != nil && field.scanMode != scanFieldGeneral {
					p.values[i] = nil
				}
			}
		}
	} else if p.layout == nil && len(p.scanners) == len(p.values) {
		// Positional scans normally have only one or two columns. Clear both
		// vectors in one loop instead of adding a memclr call to every row.
		p.releaseScalarDestinations()
		return
	} else {
		clear(p.values)
	}

	for i := range p.scanners {
		p.scanners[i].Value = nil
	}
	for i := range p.fieldScanners {
		p.fieldScanners[i].value = reflect.Value{}
	}
}

// BorrowPlan prepares synchronous collection scanning. Release it on every exit,
// including panic. Types and option slices must stay immutable until Release.
// native is true only for database/sql sources, whose Scan cannot retain targets.
func BorrowPlan(source func(...any) error, columns []string, types []reflect.Type, options ScanOptions, native bool) (*Plan, error) {
	p := scanPlanPool.Get().(*Plan)
	if _, err := initRowScanPlan(p, columns, types, options); err != nil {
		Release(p)
		return nil, err
	}

	p.source, p.nativeSource = source, native
	return p, nil
}
