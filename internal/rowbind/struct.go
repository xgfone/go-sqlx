// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package rowbind

import (
	"errors"
	"fmt"
	"reflect"
)

const (
	scanFieldUnsupported uint8 = iota
	scanFieldGeneral
	scanFieldCustom
)

func fieldScanMode(t reflect.Type) uint8 {
	pt := reflect.PointerTo(t)
	if pt.Implements(_scannertype) {
		return scanFieldCustom
	}

	if IsScalarDestination(pt) {
		return scanFieldGeneral
	}

	return scanFieldUnsupported
}

// structScanType resolves a single struct destination; a nil type selects
// positional scalar scanning, whose destination validation happens separately.
func structScanType(types []reflect.Type) (reflect.Type, error) {
	if len(types) != 1 || types[0] == nil || IsScalarDestination(types[0]) {
		return nil, nil
	}

	t := types[0]
	if t.Kind() != reflect.Pointer {
		return nil, errors.New("sqlx: expected pointer destination")
	}

	t, err := indirectType(t)
	if err != nil {
		return nil, err
	}

	if t.Kind() == reflect.Struct {
		return t, nil
	}
	return nil, nil
}

func prepareStructLayout(columns []string, t reflect.Type, options ScanOptions) (*structScanLayout, error) {
	m, err := Describe(t)
	if err != nil {
		return nil, err
	}

	var flags structScanFlags
	if options.IgnoreUnknownColumns {
		flags |= scanIgnoreUnknown
	}
	if options.NestedPointers == NilNullNestedPointers {
		flags |= scanNullParents
	}

	return m.scanLayout(t, columns, flags)
}

func initStructScanPlan(p *scanPlan, columns []string, dstType, t reflect.Type, options ScanOptions) (*scanPlan, error) {
	layout, err := prepareStructLayout(columns, t, options)
	if err != nil {
		return nil, err
	}
	return initStructLayout(p, layout, dstType, options), nil
}

func initStructLayout(p *scanPlan, layout *structScanLayout, dstType reflect.Type, options ScanOptions) *scanPlan {
	count := len(layout.columns)
	p.initStorage(count, nil, options)
	p.structDstType = dstType
	p.layout = layout
	p.fieldScanners = reuseScanStorage(p.fieldScanners, count)
	for i, field := range layout.fields {
		p.fieldScanners[i] = fieldScanner{options: &p.options}
		if field != nil {
			p.fieldScanners[i].setter = field.setter
		}

		if len(layout.groups) == 0 {
			if field == nil {
				p.values[i] = discardScanner{}
			} else if field.scanMode == scanFieldGeneral {
				p.values[i] = &p.fieldScanners[i]
			}
		}
	}

	if len(layout.groups) > 0 {
		p.captured = reuseScanStorage(p.captured, count)
		limit := nullableCaptureBytes / max(count, 1)
		for i := range p.captured {
			p.captured[i] = nullableCaptureScanner{limit: limit}
			p.values[i] = &p.captured[i]
		}
		p.skip = reuseScanStorage(p.skip, count)
	}

	return p
}

func structDestination(dst any) (reflect.Value, error) {
	v := reflect.ValueOf(dst)
	if !v.IsValid() || v.Kind() != reflect.Pointer || v.IsNil() {
		return v, errors.New("sqlx: expected non-nil pointer to struct")
	}

	t, err := indirectType(v.Type())
	if err != nil {
		return v, err
	}

	if t.Kind() != reflect.Struct {
		return v, errors.New("sqlx: expected pointer to struct")
	}

	for v.Kind() == reflect.Pointer {
		if v.IsNil() {
			if !v.CanSet() {
				return v, errors.New("sqlx: unwritable struct pointer")
			}
			v.Set(reflect.New(v.Type().Elem()))
		}
		v = v.Elem()
	}

	if v.Kind() != reflect.Struct {
		return v, errors.New("sqlx: expected pointer to struct")
	}

	return v, nil
}

func (p *scanPlan) mapStruct(v reflect.Value, adapt bool) error {
	for i, f := range p.layout.fields {
		if f == nil {
			if !adapt {
				p.values[i] = discardScanner{}
			}
			continue
		}

		var fv reflect.Value
		if len(f.Indexes) == 1 {
			fv = v.Field(f.Indexes[0])
		} else {
			var err error
			fv, err = FieldValue(v, f.Indexes, true)
			if err != nil {
				return err
			}
		}

		if !fv.CanAddr() || !fv.CanSet() {
			return fmt.Errorf("sqlx: field %q is not writable", p.layout.columns[i])
		}

		if adapt && f.scanMode == scanFieldGeneral {
			p.fieldScanners[i].value = fv
		} else {
			p.values[i] = fv.Addr().Interface()
		}
	}
	return nil
}

func (p *scanPlan) scanStruct(scan func(...any) error, dst any) error {
	v := reflect.ValueOf(dst)
	if v.IsNil() {
		return errors.New("sqlx: expected non-nil pointer to struct")
	}

	for v.Kind() == reflect.Pointer {
		if v.IsNil() {
			v.Set(reflect.New(v.Type().Elem()))
		}
		v = v.Elem()
	}

	if len(p.layout.groups) == 0 {
		if err := p.mapStruct(v, true); err != nil {
			return err
		}
		return scan(p.values...)
	}

	return p.scanNullableStruct(scan, v)
}

// ScanColumnsToStruct is a low-level field mapper: it supplies field addresses
// to scan without adapting scalar conversions. Unknown/duplicate columns are
// errors. The callback borrows its destination slice for the duration of the
// call; it must clone the slice before retaining it or any subslice. The cloned
// entries refer to the caller's fields. Scratch is released on return or panic.
// Use [Rows.Scan] or [WithScan] for conversion policies and cached plans.
func ScanColumnsToStruct(scan func(...any) error, columns []string, dst any) error {
	if scan == nil {
		return errors.New("sqlx: nil scan function")
	}

	v, err := structDestination(dst)
	if err != nil {
		return err
	}

	// The low-level mapper may be used with custom conversion functions for
	// fields that database/sql cannot scan itself (e.g. a valuer-only struct).
	m, err := Describe(v.Type())
	if err != nil {
		return err
	}

	layout, err := m.scanLayout(v.Type(), columns, scanRawFields)
	if err != nil {
		return err
	}

	p := scanPlanPool.Get().(*scanPlan)
	defer releasePlan(p)

	// Raw mapping needs field addresses only, without conversion scanners.
	p.initStorage(len(columns), nil, ScanOptions{})
	p.layout = layout

	if err := p.mapStruct(v, false); err != nil {
		return err
	}

	return scan(p.values...)
}
