// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"errors"
	"fmt"
	"reflect"
	"slices"

	"github.com/xgfone/go-sqlx/internal/rowbind"
)

// InsertPlan is an immutable field-extraction plan for a struct or pointer to
// struct T. CompileInsert creates a plan; its zero value is not usable. A plan
// may be shared across goroutines using separate builders and safe input values.
// It retains no input rows, DB, dialect, SQL or driver.Valuer results.
type InsertPlan[T any] struct {
	model    reflect.Type
	fields   []structInsertField
	columns  []string
	explicit bool

	directMode insertDirectMode
}

type insertDirectMode uint8

const (
	insertDirectNone     insertDirectMode = iota // Read every field through field.read.
	insertDirectExplicit                         // Direct reads require explicit columns.
	insertDirectAlways                           // Direct reads support implicit columns too.
)

// CompileInsert resolves mapped columns, field paths and value capabilities.
// With no columns it selects all mapped fields in declaration order; omitted
// zero values use DEFAULT, as with Structs. Explicit columns select and order
// fields and include zero values. Columns are copied and must be unique.
// T must be a struct (except time.Time) or a pointer chain to one, not an interface.
// Compilation does not read values or call IsZero or driver.Valuer.Value.
func CompileInsert[T any](columns ...string) (plan *InsertPlan[T], err error) {
	defer func() {
		if r := recover(); r != nil {
			plan, err = nil, insertPlanError(r)
		}
	}()

	model, err := rowbind.StructType(reflect.TypeFor[T]())
	if err != nil {
		return nil, err
	}

	b := Insert()
	if len(columns) != 0 {
		validateColumnNames(columns)
		b.Columns(columns...)
	}

	fields := b.structInsertFields(model, nil, make(map[string]int))

	if len(b.columns) == 0 {
		b.columns = make([]string, len(fields))
		for i, f := range fields {
			b.columns[i] = f.field.Column
		}
		validateColumnNames(b.columns)
	}

	plan = &InsertPlan[T]{
		model:    model,
		fields:   fields,
		columns:  b.columns,
		explicit: b.explicitColumns,
	}
	plan.prepareDirectFields()
	return plan, nil
}

// Cache whether field reads need only static value paths. Keep the whole-row
// box in AppendTo: it preserves snapshots and avoids boxing every field.
func (p *InsertPlan[T]) prepareDirectFields() {
	p.directMode = insertDirectAlways
	for _, f := range p.fields {
		if f.flags&(insertPointer|insertCopyValuer) != 0 || f.field.PointerParent {
			p.directMode = insertDirectNone
			break
		}
		if f.field.IgnoreZero {
			p.directMode = insertDirectExplicit
		}
	}
}

// AppendTo snapshots rows into b's VALUES batch. Existing columns must match the
// plan's order exactly; existing positional rows must have the same width.
// Empty builders adopt the plan's columns. Explicit columns on either the plan
// or builder include zero values, and an explicit plan marks the builder's
// columns explicit for subsequent Struct/Structs calls.
//
// A nil/uncompiled plan, nil builder or existing builder error always returns
// an error. Empty slices otherwise leave the builder unchanged. Nonempty input
// rejects incompatible columns, INSERT SELECT/DEFAULT VALUES sources, nil model
// pointers and failing field reads. A failed call appends no rows, changes no
// columns and does not set the builder's sticky error. User IsZero side
// effects cannot be rolled back. The builder must not be used concurrently or
// reentered by an IsZero method.
//
// Values are read during AppendTo, not Build/Exec. Copies are shallow, just as
// with Structs: pointer/slice members may still share objects with the caller.
// A value field whose pointer implements driver.Valuer gets an independent
// addressable copy; an existing pointer Valuer remains shared. Value is called
// later by the driver. AppendTo does not validate dialect-specific SQL features.
func (p *InsertPlan[T]) AppendTo(b *InsertBuilder, rows []T) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = insertPlanError(r)
		}
	}()

	if p == nil || p.model == nil {
		return errors.New("sqlx: uncompiled insert plan")
	}
	if b == nil {
		return errors.New("sqlx: nil INSERT builder")
	}
	if b.err != nil {
		return b.err
	}
	if len(rows) == 0 {
		return nil
	}
	if b.source != nil || b.defaults {
		return errors.New("sqlx: insert plan requires a VALUES source")
	}
	if (len(b.columns) != 0 || b.explicitColumns) && !slices.Equal(b.columns, p.columns) {
		return errors.New("sqlx: insert plan columns differ from builder columns")
	}

	width := len(p.fields)
	if b.values.rows != 0 && (b.values.width != width || b.values.inconsistent()) {
		return errors.New("sqlx: insert plan row width differs from existing VALUES")
	}

	defer b.values.discardPending()
	cells := b.values.nextRows(len(rows), width)
	explicit := p.explicit || b.explicitColumns
	direct := p.directMode == insertDirectAlways ||
		p.directMode == insertDirectExplicit && explicit
	for i := range rows {
		// A value model is boxed once per row, preserving Structs' snapshot
		// before any user IsZero method runs. Pointer models keep their identity.
		v := reflect.ValueOf(rows[i])
		for v.Kind() == reflect.Pointer {
			if v.IsNil() {
				return fmt.Errorf("sqlx: nil struct pointer at row %d", i)
			}
			v = v.Elem()
		}

		row := cells[i*width : (i+1)*width]
		if direct {
			for _, field := range p.fields {
				indexes := field.field.Indexes
				if len(indexes) == 1 {
					row[field.column] = v.Field(indexes[0]).Interface()
				} else {
					row[field.column] = v.FieldByIndex(indexes).Interface()
				}
			}
		} else {
			for _, field := range p.fields {
				row[field.column] = field.read(v, explicit)
			}
		}
	}

	if len(b.columns) == 0 {
		b.columns = slices.Clone(p.columns)
	}

	b.explicitColumns = explicit
	b.values.commitRows(len(rows), width)
	return nil
}

func insertPlanError(r any) error {
	if err, ok := r.(error); ok {
		return fmt.Errorf("sqlx: %w", err)
	}
	return fmt.Errorf("sqlx: %v", r)
}

type insertFieldFlags uint8

const (
	insertPointer insertFieldFlags = 1 << iota
	insertCopyValuer
	insertZeroMethod
)

var insertZeroType = reflect.TypeFor[interface{ IsZero() bool }]()

func newStructInsertField(field *rowbind.Field, column int) structInsertField {
	t := field.Type
	var flags insertFieldFlags
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
