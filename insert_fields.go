// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"fmt"
	"reflect"

	"github.com/xgfone/go-sqlx/internal/rowbind"
)

// Keep field references in evaluation order and write directly to their target columns.
type structInsertField struct {
	field  *rowbind.Field
	flags  insertFieldFlags
	column int
}

type insertFieldFlags uint8

const (
	insertPointer insertFieldFlags = 1 << iota
	insertCopyValuer
	insertZeroMethod
	insertStringLimit
)

var insertZeroType = reflect.TypeFor[interface{ IsZero() bool }]()

func insertField(v reflect.Value, field *rowbind.Field) any {
	if !v.IsValid() {
		return nil
	}

	if v.Kind() == reflect.Pointer && v.IsNil() {
		return nil
	}

	if !v.CanInterface() {
		panic("cannot read unexported field")
	}
	if field.StringLimit != nil {
		return insertLimitedString(v, field)
	}

	if v.Type().Implements(_valuertype) {
		return v.Interface()
	}

	if reflect.PointerTo(v.Type()).Implements(_valuertype) {
		copy := reflect.New(v.Type())
		copy.Elem().Set(v)
		return copy.Interface()
	}

	return v.Interface()
}

func insertLimitedString(v reflect.Value, field *rowbind.Field) any {
	for v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return nil
		}
		v = v.Elem()
	}

	s, err := field.StringLimit.ApplyString(v.String())
	if err != nil {
		panic(fmt.Errorf("sqlx: insert column %q: %w", field.Column, err))
	}

	if len(s) == v.Len() {
		// An unchanged field can reuse its existing interface storage. Changed
		// values and pointer fields are snapshots, never writes to the model.
		return v.Interface()
	}

	return s
}

// Reuse projection storage across model changes. Column indexes are initialized
// once per batch; neither the projection nor the index retains source values.
func (b *InsertBuilder) structInsertFields(t reflect.Type, projection []structInsertField, columns map[string]int) []structInsertField {
	meta, err := rowbind.Describe(t)
	if err != nil {
		panic(err)
	}

	fields := meta.Fields()
	n := len(fields)
	if b.explicitColumns {
		n = len(b.columns)
	}
	if n == 0 {
		panic("empty named row; use DefaultValues explicitly")
	}
	if !b.explicitColumns && len(b.columns) != 0 && n != len(b.columns) {
		panic("named row columns do not match")
	}

	if cap(projection) < n {
		projection = make([]structInsertField, n)
	} else {
		projection = projection[:n]
	}

	// The first implicit model already has the desired column order.
	if len(b.columns) == 0 {
		for i := range fields {
			projection[i] = newStructInsertField(&fields[i], i)
		}
		return projection
	}

	initializeColumns := len(columns) == 0
	// Validate using target order, preserving the missing-column diagnostic.
	for i, column := range b.columns {
		field := meta.Field(column)
		if field == nil {
			if !b.explicitColumns {
				panic(fmt.Sprintf("missing column %q", column))
			}
			panic(fmt.Sprintf("unknown struct column %q", column))
		}

		if initializeColumns {
			if _, exists := columns[column]; exists {
				panic(fmt.Sprintf("duplicate column %q", column))
			}
			columns[column] = i
		}

		if b.explicitColumns {
			projection[i] = newStructInsertField(field, i)
		}
	}

	if !b.explicitColumns {
		// IsZero may be user-defined: evaluate implicit fields in declaration order.
		for i := range fields {
			projection[i] = newStructInsertField(&fields[i], columns[fields[i].Column])
		}
	}

	return projection
}

func newStructInsertField(field *rowbind.Field, column int) structInsertField {
	t := field.Type
	var flags insertFieldFlags
	if field.StringLimit != nil {
		flags |= insertStringLimit
	}
	if t.Kind() == reflect.Pointer {
		flags |= insertPointer
	}
	if !t.Implements(_valuertype) && reflect.PointerTo(t).Implements(_valuertype) {
		flags |= insertCopyValuer
	}

	// Interface fields may expose IsZero on their dynamic value. Pointer-only
	// methods of value fields are deliberately not called, matching isZero.
	if field.IgnoreZero && (t.Kind() == reflect.Interface || t.Implements(insertZeroType)) {
		flags |= insertZeroMethod
	}

	return structInsertField{
		field:  field,
		flags:  flags,
		column: column,
	}
}

func (f structInsertField) read(model reflect.Value, explicit bool) any {
	var v reflect.Value
	if len(f.field.Indexes) == 1 {
		v = model.Field(f.field.Indexes[0])
	} else {
		var err error
		v, err = rowbind.FieldValue(model, f.field.Indexes, false)
		if err != nil {
			panic(err)
		}
	}

	if !explicit && f.field.IgnoreZero && f.zero(v) {
		return Default()
	}
	if !v.IsValid() || f.flags&insertPointer != 0 && v.IsNil() {
		return nil
	}
	if !v.CanInterface() {
		panic("cannot read unexported field")
	}
	if f.flags&insertStringLimit != 0 {
		return insertLimitedString(v, f.field)
	}

	if f.flags&insertCopyValuer != 0 {
		copy := reflect.New(v.Type())
		copy.Elem().Set(v)
		return copy.Interface()
	}

	return v.Interface()
}

func (f structInsertField) zero(v reflect.Value) bool {
	if !v.IsValid() || v.IsZero() {
		return true
	}
	if f.flags&insertZeroMethod != 0 {
		if z, ok := v.Interface().(interface{ IsZero() bool }); ok {
			return z.IsZero()
		}
	}
	return false
}
