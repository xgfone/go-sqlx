// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"errors"
	"reflect"

	"github.com/xgfone/go-sqlx/internal/rowbind"
)

// SliceRowsBinder binds any pointer to a slice. It uses typed paths for common
// scalars and reflection for other element types. It never guesses map semantics.
type SliceRowsBinder struct{}

func (SliceRowsBinder) Prepare(dst any, options BindOptions) (RowsBinding, error) {
	if binder := builtinSliceBinders[reflect.TypeOf(dst)]; binder != nil {
		return binder.Prepare(dst, options)
	}

	v := reflect.ValueOf(dst)
	if !v.IsValid() || v.Kind() != reflect.Pointer || v.Type().Elem().Kind() != reflect.Slice {
		return unsupportedBinder("sqlx.SliceRowsBinder", dst)
	}

	if v.IsNil() {
		return nil, errors.New("sqlx: nil slice destination")
	}

	if err := validateSliceOptions(options); err != nil {
		return nil, err
	}

	types := []reflect.Type{reflect.PointerTo(v.Type().Elem().Elem())}
	mapping, err := rowbind.Prepare(options.Columns, types, options.ScanOptions)
	if err != nil {
		return nil, err
	}

	return &sliceRowsBinding{
		mapping:  mapping,
		pointer:  v,
		capacity: options.capacity(),
		mode:     options.Mode,
	}, nil
}

type sliceRowsBinding struct {
	mapping  rowbind.Mapping
	pointer  reflect.Value
	staged   reflect.Value
	args     [1]any
	capacity int
	mode     BindMode
}

func (b *sliceRowsBinding) Commit() {
	b.pointer.Elem().Set(b.staged)
}

func (b *sliceRowsBinding) Scan(cursor RowCursor) error {
	return b.mapping.WithScan(cursor, func(scan RowScanFunc, _ rowbind.Reuse) error {
		return b.scanRows(cursor, scan)
	})
}

func (b *sliceRowsBinding) scanRows(scanner RowCursor, scan RowScanFunc) error {
	defer clear(b.args[:])
	t := b.pointer.Elem().Type()

	base := 0
	if b.mode == BindAppend {
		base = b.pointer.Elem().Len()
	}
	if base > int(^uint(0)>>1)-b.capacity {
		return errors.New("sqlx: slice capacity overflow")
	}

	staged := reflect.New(t).Elem()

	row := 0
	for scanner.Next() {
		row++
		if row == 1 {
			staged.Set(reflect.MakeSlice(t, base+b.capacity, base+b.capacity))
			if base > 0 {
				reflect.Copy(staged, b.pointer.Elem())
			}
		}

		// Expose allocated slots once per growth; publish the actual length
		// only after successful scanning. No per-row Grow/SetLen is needed.
		if base+row > staged.Len() {
			staged.Grow(1)
			staged.SetLen(staged.Cap())
		}

		b.args[0] = staged.Index(base + row - 1).Addr().Interface()
		if err := scan(b.args[:]...); err != nil {
			return &BindError{row, err}
		}
	}

	if err := scanner.Err(); err != nil {
		return &BindError{row + 1, err}
	}

	if row == 0 {
		if b.mode == BindAppend {
			staged.Set(b.pointer.Elem())
		} else {
			staged.Set(reflect.MakeSlice(t, 0, 0))
		}
	} else {
		staged.SetLen(base + row)
	}

	b.staged = staged
	return nil
}

func validateSliceOptions(options BindOptions) error {
	if err := options.validate(); err != nil {
		return err
	}
	if options.Mode == BindMerge {
		return errors.New("sqlx: Merge requires a map destination")
	}
	return nil
}

// NewSliceRowsBinder avoids reflection for slice growth and element storage.
// Element conversion and struct field mapping still use the shared scan engine.
func NewSliceRowsBinder[S ~[]T, T any]() RowsBinder { return typedSliceRowsBinder[S, T]{} }

type typedSliceRowsBinder[S ~[]T, T any] struct{}

func (b typedSliceRowsBinder[S, T]) Prepare(dst any, options BindOptions) (RowsBinding, error) {
	state, err := b.prepare(dst, options)
	if err != nil {
		return nil, err
	}
	return state, nil
}

func (typedSliceRowsBinder[S, T]) prepare(dst any, options BindOptions) (*typedSliceBinding[S, T], error) {
	pointer, ok := dst.(*S)
	if !ok {
		return nil, UnsupportedTypeError{Name: "sqlx.NewSliceRowsBinder", Type: gettype(dst)}
	}
	if pointer == nil {
		return nil, errors.New("sqlx: nil slice destination")
	}
	if err := validateSliceOptions(options); err != nil {
		return nil, err
	}

	types := []reflect.Type{reflect.TypeFor[*T]()}
	mapping, err := rowbind.Prepare(options.Columns, types, options.ScanOptions)
	if err != nil {
		return nil, err
	}

	return &typedSliceBinding[S, T]{
		mapping:  mapping,
		pointer:  pointer,
		capacity: options.capacity(),
		mode:     options.Mode,
	}, nil
}

type typedSliceBinding[S ~[]T, T any] struct {
	mapping  rowbind.Mapping
	pointer  *S
	staged   S
	args     [1]any
	capacity int
	mode     BindMode
}

func (b *typedSliceBinding[S, T]) Commit() { *b.pointer = b.staged }

func (b *typedSliceBinding[S, T]) Scan(cursor RowCursor) error {
	return b.mapping.WithScan(cursor, func(scan RowScanFunc, _ rowbind.Reuse) error {
		return b.scanRows(cursor, scan)
	})
}

func (b *typedSliceBinding[S, T]) scanRows(scanner RowCursor, scan RowScanFunc) error {
	defer clear(b.args[:])

	b.staged = make(S, 0)
	base := 0
	if b.mode == BindAppend {
		base = len(*b.pointer)
	}
	if base > int(^uint(0)>>1)-b.capacity {
		return errors.New("sqlx: slice capacity overflow")
	}

	var zero T
	row := 0
	for scanner.Next() {
		row++
		if row == 1 {
			b.staged = make(S, base, base+b.capacity)
			copy(b.staged, (*b.pointer)[:base])
		}

		b.staged = append(b.staged, zero)
		b.args[0] = &b.staged[len(b.staged)-1]
		if err := scan(b.args[:]...); err != nil {
			return &BindError{row, err}
		}
	}

	if err := scanner.Err(); err != nil {
		return &BindError{row + 1, err}
	}
	if row == 0 && b.mode == BindAppend {
		b.staged = *b.pointer
	}
	return nil
}
