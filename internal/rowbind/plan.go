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

// scanPlan is mutable per-result scan state. It must not be copied or used concurrently.
// Fields, layouts and setters remain private so callers depend only on operations.
type scanPlan struct {
	options       ScanOptions
	layout        *structScanLayout
	structDstType reflect.Type
	types         []reflect.Type
	scanners      []GeneralScanner
	fieldScanners []fieldScanner
	values        []any
	wrapped       []bool
	captured      []captureScanner
	skip          []bool

	visitBound bool // Only Visit may keep its private flat target bound between rows.
}

type nullStructGroup struct {
	path    []int
	columns []int
}

type captureScanner struct{ value any }

func (s *captureScanner) Scan(value any) error {
	// Conversion happens after the driver's Scan returns, when cancellation
	// may already have closed the cursor and invalidated its byte buffers.
	if data, ok := value.([]byte); ok {
		value = slices.Clone(data)
	}
	s.value = value
	return nil
}

type discardScanner struct{}

func (discardScanner) Scan(any) error { return nil }

func initRowScanPlan(p *scanPlan, columns []string, types []reflect.Type, options ScanOptions) (*scanPlan, error) {
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

func initScalarScanPlan(p *scanPlan, columns []string, types []reflect.Type, options ScanOptions) (*scanPlan, error) {
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
		wrapped, err := scalarDestinationWrapper(t, i)
		if err != nil {
			return nil, err
		}
		p.wrapped[i] = wrapped
	}

	return p, nil
}

func scalarDestinationWrapper(t reflect.Type, column int) (bool, error) {
	custom := t != nil && t.Implements(_scannertype) && t.Kind() != reflect.Interface
	if t == nil || (!custom && (t.Kind() != reflect.Pointer || !IsScalarDestination(t))) {
		return false, fmt.Errorf("sqlx: unsupported scalar destination %v at column %d", t, column)
	}
	return !custom, nil
}

func reuseScanStorage[T any](values []T, count int) []T {
	if cap(values) < count {
		return make([]T, count)
	}
	return values[:count]
}

func (p *scanPlan) initStorage(count int, types []reflect.Type, options ScanOptions) {
	p.types = reuseScanStorage(p.types, len(types))
	copy(p.types, types)

	p.values = reuseScanStorage(p.values, count)
	p.scanners = p.scanners[:0]
	p.fieldScanners = p.fieldScanners[:0]
	p.options = options

	p.layout = nil
	p.structDstType = nil
	p.visitBound = false
}

// Scans with a synchronous operation or explicit Reset borrow this scratch pool.
// Large plans are discarded, and no result, configuration or destination is
// retained. sync.Pool may drop storage at any GC; correctness never relies on it.
var scanPlanPool = sync.Pool{New: func() any { return &scanPlan{} }}

// releasePlan clears all references and returns a borrowed plan to the scratch pool.
// The caller must not use the plan after release.
func releasePlan(p *scanPlan) {
	p.releaseDestinations()
	clear(p.values)
	clear(p.types)
	clear(p.scanners)
	clear(p.fieldScanners)
	clear(p.captured)
	p.options = ScanOptions{}
	p.layout = nil
	p.structDstType = nil
	p.visitBound = false
	if cap(p.types) <= 64 && cap(p.values) <= 64 && cap(p.scanners) <= 64 && cap(p.fieldScanners) <= 64 {
		scanPlanPool.Put(p)
	}
}

// ScanStruct borrows scratch storage for one synchronous struct scan.
//
// dst must contain exactly one struct pointer. The scan callback must clone its
// argument slice before retaining it; temporary adapters remain call-scoped.
func ScanStruct(scan func(...any) error, columns []string, dst []any, options ScanOptions) error {
	if len(dst) != 1 || dst[0] == nil || scan == nil {
		return errors.New("sqlx: expected one struct destination and a scan function")
	}

	p := scanPlanPool.Get().(*scanPlan)
	defer releasePlan(p)

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

// matches reports whether the destination count and types match this plan.
func (p *scanPlan) matches(dst []any) bool {
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
func (p *scanPlan) Scan(scan func(...any) error, dst []any) error {
	if !p.matches(dst) {
		return errors.New("sqlx: prepared scan destination types changed")
	}
	return p.scanValues(scan, dst)
}

// scanValues scans destinations whose count and types are already validated.
// The scan function must be non-nil. Use Scan when types can change between rows.
func (p *scanPlan) scanValues(scan func(...any) error, dst []any) error {
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
func (p *scanPlan) releaseScalarDestinations() {
	for i := range p.values {
		p.values[i] = nil
		p.scanners[i].Value = nil
	}
}

func (p *scanPlan) releaseDestinations() {
	if p.layout != nil && len(p.layout.groups) == 0 {
		// Built-in wrappers only point into this plan. Keep that stable argument
		// vector across rows; clear the actual destinations below. Direct fields,
		// including standard nullable Scanners, must release caller addresses.
		if p.layout.directTargets {
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
