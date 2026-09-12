// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"fmt"
	"reflect"

	"github.com/xgfone/go-sqlx/internal/rowbind"
)

// Struct appends one row. Explicit Columns select fields and include zero values.
// Otherwise omission tags are honored and every row must have the same column set.
func (b *InsertBuilder) Struct(s any) *InsertBuilder {
	b.mutate(func() { b.appendStruct(s) })
	return b
}

func (b *InsertBuilder) appendStruct(s any) {
	v, e := rowbind.StructValue(s)
	if e != nil {
		panic(e)
	}

	meta, e := rowbind.Describe(v.Type())
	if e != nil {
		panic(e)
	}

	var row []ColumnValue
	if b.explicitColumns {
		for _, col := range b.columns {
			f := meta.Field(col)
			if f == nil {
				panic(fmt.Sprintf("unknown struct column %q", col))
			}

			fv, e := rowbind.FieldValue(v, f.Indexes, false)
			if e != nil {
				panic(e)
			}

			row = append(row, ColValue(col, insertField(fv)))
		}
	} else {
		for _, f := range meta.Fields() {
			fv, e := rowbind.FieldValue(v, f.Indexes, false)
			if e != nil {
				panic(e)
			}

			if f.IgnoreZero && (!fv.IsValid() || isZero(fv)) {
				continue
			}

			row = append(row, ColValue(f.Column, insertField(fv)))
		}
	}
	b.appendNamedRow(row)
}

func insertField(v reflect.Value) any {
	if !v.IsValid() {
		return nil
	}

	if v.Kind() == reflect.Pointer && v.IsNil() {
		return nil
	}

	if !v.CanInterface() {
		panic("cannot read unexported field")
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

// Structs appends a slice of structs or pointers using a fixed set of mapped columns.
// Without explicit Columns, zero fields tagged omitempty or omitzero use SQL DEFAULT
// instead of being omitted. The dialect must support DEFAULT in VALUES when needed.
// Explicit Columns select fields in that order and include their actual zero values.
// Empty slices append no rows; an otherwise empty insert still fails Build.
func (b *InsertBuilder) Structs(slice any) *InsertBuilder {
	b.mutate(func() { b.appendStructs(slice) })
	return b
}

func (b *InsertBuilder) appendStructs(slice any) {
	v := reflect.ValueOf(slice)
	if !v.IsValid() || v.Kind() != reflect.Slice {
		panic("Structs requires a slice")
	}
	if v.Len() == 0 {
		return
	}

	count := v.Len()

	// Resolve field order once for a homogeneous batch. Interface slices
	// may contain different models, so refresh it when the model changes.
	var model reflect.Type
	// Small projections stay on the stack; larger ones grow once per batch.
	var storage [16]structInsertField
	fields := storage[:0]
	columnIndexes := make(map[string]int)
	defer b.values.discardPending()
	for i := 0; i < v.Len(); i++ {
		v, err := rowbind.StructValue(v.Index(i).Interface())
		if err != nil {
			panic(err)
		}

		if v.Type() != model {
			fields = b.structInsertFields(v.Type(), fields, columnIndexes)
			if model == nil {
				b.values.grow(count, len(fields))
			}
			model = v.Type()
		}

		// Store the final argument row directly, without named-value
		// intermediates, a per-row map, or a second argument-slice copy.
		row := b.values.nextRow(len(fields))
		for _, entry := range fields {
			row[entry.column] = entry.read(v, b.explicitColumns)
		}

		if len(b.columns) == 0 {
			b.columns = make([]string, len(fields))
			for j, entry := range fields {
				b.columns[j] = entry.field.Column
			}
		}

		b.values.commitRow(len(row))
		if b.err != nil {
			return
		}
	}
}

// Keep field references in evaluation order and write directly to their target columns.
type structInsertField struct {
	field  *rowbind.Field
	flags  insertFieldFlags
	column int
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
