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

func initStructScanPlan(p *Plan, columns []string, dstType, t reflect.Type, options ScanOptions) (*Plan, error) {
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

	layout, err := m.scanLayout(t, columns, flags)
	if err != nil {
		return nil, err
	}

	p.initStorage(len(columns), nil, options)
	p.structDstType = dstType
	p.layout = layout
	p.fieldScanners = reuseScanStorage(p.fieldScanners, len(columns))
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
		p.captured = reuseScanStorage(p.captured, len(columns))
		p.skip = reuseScanStorage(p.skip, len(columns))
	}

	return p, nil
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

func (p *Plan) mapStruct(v reflect.Value, adapt bool) error {
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

func (p *Plan) scanStruct(scan func(...any) error, dst any) error {
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

	// The nullable-parent policy needs source NULL information. Buffer only this
	// opt-in path, and convert before advancing the driver or releasing its bytes.
	for i := range p.values {
		p.values[i] = &p.captured[i]
	}

	defer func() {
		for i := range p.captured {
			p.captured[i].value = nil
		}
	}()

	if err := scan(p.values...); err != nil {
		return err
	}

	clear(p.skip)
	for _, g := range p.layout.groups {
		allNull := true
		for _, i := range g.columns {
			if p.captured[i].value != nil {
				allNull = false
				break
			}
		}

		if !allNull {
			continue
		}

		fv, err := FieldValue(v, g.path, false)
		if err != nil {
			return err
		}

		if fv.IsValid() {
			if !fv.CanSet() {
				return errors.New("sqlx: unwritable nested pointer")
			}
			fv.SetZero()
		}

		for _, i := range g.columns {
			p.skip[i] = true
		}
	}

	for i, f := range p.layout.fields {
		if p.skip[i] || f == nil {
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

		p.fieldScanners[i].value = fv
		if err := p.fieldScanners[i].Scan(p.captured[i].value); err != nil {
			return fmt.Errorf("sqlx: column %d (%q): %w", i, p.layout.columns[i], err)
		}
	}

	return nil
}

// ScanColumnsToStruct is a low-level field mapper: it supplies field addresses
// to scan without adapting scalar conversions. Unknown/duplicate columns are
// errors. Use Rows.Scan or PrepareScan for conversion policies and cached plans.
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

	// The callback may retain its argument slice, so this public mapper owns
	// fresh destination storage. Only the immutable layout is shared.
	p := &Plan{layout: layout, values: make([]any, len(columns))}

	if err := p.mapStruct(v, false); err != nil {
		return err
	}

	return scan(p.values...)
}
