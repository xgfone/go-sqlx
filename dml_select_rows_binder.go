// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"errors"
	"fmt"
	"maps"
	"reflect"
	"slices"

	"github.com/xgfone/go-sqlx/internal/rowbind"
)

// UnsupportedTypeError is returned by Prepare when a binder does not recognize
// a destination type. A recognized but invalid destination returns another error.
type UnsupportedTypeError struct {
	Name string
	Type string
}

func (e UnsupportedTypeError) Error() string {
	return fmt.Sprintf("%s: unsupported type %s", e.Name, e.Type)
}

// IsUnsupportedTypeError checks if the error is a UnsupportedTypeError.
func IsUnsupportedTypeError(err error) bool {
	if _, ok := errors.AsType[UnsupportedTypeError](err); ok {
		return true
	}

	_, ok := errors.AsType[*UnsupportedTypeError](err)
	return ok
}

// RowsBinder prepares an immutable mapping from the supplied columns and scan
// options without accessing a cursor or borrowing execution scratch. It must not
// mutate dst. Once it succeeds, no fallback is attempted, whatever Scan returns.
// Implementations may be shared by concurrent queries; each prepared binding
// must own its state and scratch storage.
type RowsBinder interface {
	Prepare(dst any, options BindOptions) (RowsBinding, error)
}

type RowsBinderFunc func(any, BindOptions) (RowsBinding, error)

func (f RowsBinderFunc) Prepare(dst any, options BindOptions) (RowsBinding, error) {
	if f == nil {
		return nil, errors.New("sqlx: nil rows binder function")
	}
	return f(dst, options)
}

// RowsBinding is a single-use staged operation. Scan reads into independent
// storage using a raw cursor matching the prepared column order and returns
// iteration errors. BindOptions supplies conversion policies. Commit publishes
// it and must not fail or perform I/O. Call Commit only after Scan and any owning
// result's Close have succeeded. Custom side effects are the binder's responsibility.
type RowsBinding interface {
	Scan(RowCursor) error
	Commit()
}

// RowsBindingFuncs adapts callbacks to a RowsBinding. Scan rejects missing
// callbacks before consuming rows. Commit may be called only after Scan succeeds.
// For allocation-sensitive binders, let the per-result state implement
// RowsBinding directly instead of creating method-value callbacks.
type RowsBindingFuncs struct {
	ScanFunc   func(RowCursor) error
	CommitFunc func()
}

func (b RowsBindingFuncs) Scan(rows RowCursor) error {
	if b.ScanFunc == nil || b.CommitFunc == nil {
		return errors.New("sqlx: incomplete rows binding")
	}
	return b.ScanFunc(rows)
}

func (b RowsBindingFuncs) Commit() { b.CommitFunc() }

// BindError adds the one-based row number to a conversion, duplicate-key, or
// iteration error. Iterator failures refer to the next row being requested.
type BindError struct {
	Row int
	Err error
}

func (e *BindError) Error() string { return fmt.Sprintf("sqlx: row %d: %v", e.Row, e.Err) }
func (e *BindError) Unwrap() error { return e.Err }

// DuplicateKeyError reports a collision under the DuplicateKeyReject policy.
type DuplicateKeyError struct{ Key any }

func (e *DuplicateKeyError) Error() string {
	return fmt.Sprintf("sqlx: duplicate map key %v", e.Key)
}

func unsupportedBinder(name string, dst any) (RowsBinding, error) {
	return nil, UnsupportedTypeError{Name: name, Type: gettype(dst)}
}

// ComposeRowsBinders tries Prepare in order. Unsupported destinations fall
// through; validation errors stop selection. Empty/all-nil chains return an
// UnsupportedTypeError. Put SliceRowsBinder last when a general fallback is wanted.
func ComposeRowsBinders(binders ...RowsBinder) RowsBinder {
	binders = slices.Clone(binders)
	return RowsBinderFunc(func(dst any, options BindOptions) (RowsBinding, error) {
		if err := options.validate(); err != nil {
			return nil, err
		}

		for _, binder := range binders {
			if nilBindingValue(binder) {
				continue
			}

			binding, err := binder.Prepare(dst, options)
			if err == nil || !IsUnsupportedTypeError(err) {
				return binding, err
			}
		}

		return unsupportedBinder("sqlx.ComposeRowsBinders", dst)
	})
}

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
	mapping, err := rowbind.Prepare(options.Columns, types, options.Scan)
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
	return b.mapping.WithScan(cursor, func(scan func(...any) error, _ rowbind.Reuse) error {
		return b.scanRows(cursor, scan)
	})
}

func (b *sliceRowsBinding) scanRows(scanner RowCursor, scan func(...any) error) error {
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
	mapping, err := rowbind.Prepare(options.Columns, types, options.Scan)
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
	return b.mapping.WithScan(cursor, func(scan func(...any) error, _ rowbind.Reuse) error {
		return b.scanRows(cursor, scan)
	})
}

func (b *typedSliceBinding[S, T]) scanRows(scanner RowCursor, scan func(...any) error) error {
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

// NewMapPairsBinder scans two positional columns as key and value. Duplicate
// keys fail by default. Only a non-nil pointer to M is accepted.
func NewMapPairsBinder[M ~map[K]V, K comparable, V any]() RowsBinder {
	return mapRowsBinder[M](false, nil, []reflect.Type{
		reflect.TypeFor[*K](),
		reflect.TypeFor[*V](),
	}, scanMapPairs[K, V])
}

func scanMapPairs[K comparable, V any](scan func(...any) error, reusable rowbind.Reuse) func() (K, V, error) {
	args := make([]any, 2)
	if reusable[0] && reusable[1] {
		var key K
		var value V
		args[0], args[1] = &key, &value
		return func() (K, V, error) {
			var zeroK K
			var zeroV V
			key, value = zeroK, zeroV
			err := scan(args...)
			return key, value, err
		}
	}

	if !reusable[0] && !reusable[1] {
		return func() (key K, value V, err error) {
			args[0], args[1] = &key, &value
			err = scan(args...)
			return
		}
	}

	// Reuse each safe side independently. A custom Scanner on the other side
	// still receives a fresh address, and may retain it after Scan returns.
	var key *K
	var value *V
	if reusable[0] {
		key = new(K)
	}
	if reusable[1] {
		value = new(V)
	}

	return func() (K, V, error) {
		k, v := key, value
		if k == nil {
			k = new(K)
		}
		if v == nil {
			v = new(V)
		}

		var zeroK K
		var zeroV V
		*k, *v = zeroK, zeroV
		args[0], args[1] = k, v
		err := scan(args...)
		return *k, *v, err
	}
}

// NewMapIndexBinder scans each row as V and computes its key. A nil key function
// is a preparation error. Duplicate keys fail by default.
func NewMapIndexBinder[M ~map[K]V, K comparable, V any](key func(V) K) RowsBinder {
	var configError error
	if key == nil {
		configError = errors.New("sqlx: nil map key function")
	}

	return mapRowsBinder[M](
		false,
		configError,
		[]reflect.Type{reflect.TypeFor[*V]()},
		func(scan func(...any) error, reusable rowbind.Reuse) func() (K, V, error) {
			args := []any{nil}
			if reusable[0] {
				var value V
				args[0] = &value
				return func() (k K, v V, err error) {
					var zero V
					value = zero
					if err = scan(args...); err == nil {
						k = key(value)
					}
					return k, value, err
				}
			}

			return func() (k K, value V, err error) {
				args[0] = &value
				if err = scan(args...); err == nil {
					k = key(value)
				}
				return
			}
		},
	)
}

// NewMapSetBinder scans each row as a set key. Duplicate keys are intentionally
// deduplicated, including during Merge. Use map[K]struct{} to express a set.
func NewMapSetBinder[M ~map[K]struct{}, K comparable]() RowsBinder {
	return mapRowsBinder[M](
		true,
		nil,
		[]reflect.Type{reflect.TypeFor[*K]()},
		func(scan func(...any) error, reusable rowbind.Reuse) func() (K, struct{}, error) {
			args := []any{nil}
			if reusable[0] {
				var key K
				args[0] = &key
				return func() (K, struct{}, error) {
					var zero K
					key = zero
					err := scan(args...)
					return key, struct{}{}, err
				}
			}

			return func() (key K, value struct{}, err error) {
				args[0] = &key
				err = scan(args...)
				return
			}
		},
	)
}

func mapRowsBinder[M ~map[K]V, K comparable, V any](
	set bool,
	configError error,
	types []reflect.Type,
	makeScan scanMaker[K, V],
) RowsBinder {
	checkKey := dynamicMapKey(reflect.TypeFor[K]())
	return RowsBinderFunc(func(dst any, options BindOptions) (RowsBinding, error) {
		pointer, ok := dst.(*M)

		if !ok {
			return unsupportedBinder("sqlx.MapRowsBinder", dst)
		}
		if pointer == nil {
			return nil, errors.New("sqlx: nil map destination")
		}
		if err := options.validate(); err != nil {
			return nil, err
		}
		if options.Mode == BindAppend {
			return nil, errors.New("sqlx: Append requires a slice destination")
		}
		if configError != nil {
			return nil, configError
		}

		mapping, err := rowbind.Prepare(options.Columns, types, options.Scan)
		if err != nil {
			return nil, err
		}

		return &mapRowsBinding[M, K, V]{
			mapping:    mapping,
			pointer:    pointer,
			makeScan:   makeScan,
			capacity:   options.capacity(),
			duplicates: options.DuplicateKeys,
			bindMode:   options.Mode,
			checkKey:   checkKey,
			set:        set,
		}, nil
	})
}

type scanMaker[K comparable, V any] func(func(...any) error, rowbind.Reuse) func() (K, V, error)

type mapRowsBinding[M ~map[K]V, K comparable, V any] struct {
	pointer    *M
	staged     M
	mapping    rowbind.Mapping
	makeScan   scanMaker[K, V]
	capacity   int
	duplicates DuplicateKeyPolicy
	bindMode   BindMode
	checkKey   bool
	set        bool
}

func (b *mapRowsBinding[M, K, V]) Commit() { *b.pointer = b.staged }

func (b *mapRowsBinding[M, K, V]) Scan(cursor RowCursor) error {
	return b.mapping.WithScan(cursor, func(scan func(...any) error, reusable rowbind.Reuse) error {
		return b.scanRows(cursor, scan, reusable)
	})
}

func (b *mapRowsBinding[M, K, V]) scanRows(scanner RowCursor, scan func(...any) error, reusable rowbind.Reuse) error {
	base := 0
	if b.bindMode == BindMerge {
		base = len(*b.pointer)
	}
	if base > int(^uint(0)>>1)-b.capacity {
		return errors.New("sqlx: map capacity overflow")
	}

	var scanRow func() (K, V, error)
	row := 0
	for scanner.Next() {
		row++
		if row == 1 {
			b.staged = make(M, base+b.capacity)
			if b.bindMode == BindMerge {
				maps.Copy(b.staged, *b.pointer)
			}
			// Empty results need neither map storage nor reusable destinations.
			scanRow = b.makeScan(scan, reusable)
		}

		key, value, err := scanRow()
		if err != nil {
			return &BindError{row, err}
		}

		if b.checkKey {
			if v := reflect.ValueOf(key); v.IsValid() && !v.Comparable() {
				return &BindError{row, fmt.Errorf("sqlx: non-comparable map key %T", key)}
			}
		}

		if !b.set && b.duplicates != DuplicateKeyLast {
			if _, exists := b.staged[key]; exists {
				if b.duplicates == DuplicateKeyReject {
					return &BindError{row, &DuplicateKeyError{key}}
				}
				continue
			}
		}

		b.staged[key] = value
	}

	if err := scanner.Err(); err != nil {
		return &BindError{row + 1, err}
	}

	if row == 0 {
		if b.bindMode == BindMerge {
			b.staged = *b.pointer
		}
		if b.staged == nil {
			b.staged = make(M)
		}
	}

	return nil
}

// A comparable type can contain interfaces whose dynamic values are not
// comparable. Ordinary scalar, pointer and interface-free aggregate keys need
// no per-row reflection or interface boxing.
func dynamicMapKey(t reflect.Type) bool {
	switch t.Kind() {
	case reflect.Interface:
		return true

	case reflect.Array:
		return t.Len() != 0 && dynamicMapKey(t.Elem())

	case reflect.Struct:
		for field := range t.Fields() {
			if dynamicMapKey(field.Type) {
				return true
			}
		}
	}
	return false
}
