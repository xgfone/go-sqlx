// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package rowbind

import (
	"errors"
	"fmt"
	"reflect"
	"slices"
)

// Deferred row input is borrowed by the converter and may be overwritten on
// the next row. Bound reusable byte storage across the entire scan operation.
const nullableCaptureBytes = 4096

type nullableCaptureScanner struct {
	value  any
	buffer []byte
	bytes  any
	limit  int
}

func (s *nullableCaptureScanner) Scan(value any) error {
	// Nullable-parent layouts defer conversion until the entire row is known.
	// Own byte values before returning to the source, which may immediately
	// reuse its buffer for another column or close on cancellation.
	if data, ok := value.([]byte); ok {
		if len(data) == 0 || len(data) > s.limit {
			// Large values are call-scoped; empty values must not retain backing
			// storage. Neither may grow the reusable buffer without a bound.
			value = slices.Clone(data)
		} else {
			if len(s.buffer) != len(data) {
				if cap(s.buffer) < len(data) {
					s.buffer = make([]byte, len(data))
				} else {
					s.buffer = s.buffer[:len(data)]
				}
				s.bytes = s.buffer[:len(data):len(data)]
			}
			copy(s.buffer, data)
			value = s.bytes
		}
	}
	s.value = value
	return nil
}

// scanNullableStruct preserves column conversion order after capturing a row.
func (p *scanPlan) scanNullableStruct(scan func(...any) error, v reflect.Value) error {
	// Capture owns any borrowed bytes before the source callback returns.
	// Conversion waits until all selected NULL-parent groups have been examined.
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
