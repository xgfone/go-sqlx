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

// RowsBinder selects a destination without access to a cursor. Prepare must not
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
// storage, validates the result and returns iteration errors. Commit publishes
// it and must not fail or perform I/O. Call Commit only after Scan and any owning
// result's Close have succeeded. Custom side effects are the binder's responsibility.
type RowsBinding interface {
	Scan(RowsScanner) error
	Commit()
}

// RowsBindingFuncs adapts callbacks to a RowsBinding. Scan rejects missing
// callbacks before consuming rows. Commit may be called only after Scan succeeds.
// For allocation-sensitive binders, let the per-result state implement
// RowsBinding directly instead of creating method-value callbacks.
type RowsBindingFuncs struct {
	ScanFunc   func(RowsScanner) error
	CommitFunc func()
}

func (b RowsBindingFuncs) Scan(rows RowsScanner) error {
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

	return &sliceRowsBinding{
		pointer:  v,
		capacity: options.capacity(),
		mode:     options.Mode,
	}, nil
}

type sliceRowsBinding struct {
	pointer  reflect.Value
	staged   reflect.Value
	capacity int
	mode     BindMode
}

func (b *sliceRowsBinding) Commit() {
	b.pointer.Elem().Set(b.staged)
}

func (b *sliceRowsBinding) Scan(scanner RowsScanner) error {
	t := b.pointer.Elem().Type()
	scan, err := prepareBindingScan(scanner, reflect.PointerTo(t.Elem()))
	if err != nil {
		return err
	}
	defer rowbind.Release(scan)

	base := 0
	if b.mode == BindAppend {
		base = b.pointer.Elem().Len()
	}
	if base > int(^uint(0)>>1)-b.capacity {
		return errors.New("sqlx: slice capacity overflow")
	}

	// The plan copies the destinations, so this argument vector can stay local.
	args := []any{nil}
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

		args[0] = staged.Index(base + row - 1).Addr().Interface()
		if err := scan.ScanCurrent(args...); err != nil {
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
	return &typedSliceBinding[S, T]{
		pointer:  pointer,
		capacity: options.capacity(),
		mode:     options.Mode,
	}, nil
}

type typedSliceBinding[S ~[]T, T any] struct {
	pointer  *S
	staged   S
	args     [1]any
	capacity int
	mode     BindMode
}

func (b *typedSliceBinding[S, T]) Commit() { *b.pointer = b.staged }

func (b *typedSliceBinding[S, T]) Scan(scanner RowsScanner) error {
	scan, err := prepareBindingScan(scanner, reflect.TypeFor[*T]())
	if err != nil {
		return err
	}

	defer rowbind.Release(scan)
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
		if err := scan.ScanCurrent(b.args[:]...); err != nil {
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
	return mapRowsBinder[M](false, nil, func(scanner RowsScanner) (func() (K, V, error), *rowbind.Plan, error) {
		scan, err := prepareBindingScan(scanner, reflect.TypeFor[*K](), reflect.TypeFor[*V]())
		if err != nil {
			return nil, nil, err
		}

		args := make([]any, 2)
		if scan.ReusableMapValues() {
			var key K
			var value V
			args[0], args[1] = &key, &value
			return func() (K, V, error) {
				var zeroK K
				var zeroV V
				key, value = zeroK, zeroV
				err := scan.ScanCurrent(args...)
				return key, value, err
			}, scan, nil
		}

		return func() (key K, value V, err error) {
			args[0], args[1] = &key, &value
			err = scan.ScanCurrent(args...)
			return
		}, scan, nil
	})
}

// NewMapIndexBinder scans each row as V and computes its key. A nil key function
// is a preparation error. Duplicate keys fail by default.
func NewMapIndexBinder[M ~map[K]V, K comparable, V any](key func(V) K) RowsBinder {
	var configError error
	if key == nil {
		configError = errors.New("sqlx: nil map key function")
	}

	return mapRowsBinder[M](false, configError, func(scanner RowsScanner) (func() (K, V, error), *rowbind.Plan, error) {
		scan, err := prepareBindingScan(scanner, reflect.TypeFor[*V]())
		if err != nil {
			return nil, nil, err
		}

		args := []any{nil}
		if scan.ReusableMapValues() {
			var value V
			args[0] = &value
			return func() (k K, v V, err error) {
				var zero V
				value = zero
				if err = scan.ScanCurrent(args...); err == nil {
					k = key(value)
				}
				return k, value, err
			}, scan, nil
		}

		return func() (k K, value V, err error) {
			args[0] = &value
			if err = scan.ScanCurrent(args...); err == nil {
				k = key(value)
			}
			return
		}, scan, nil
	})
}

// NewMapSetBinder scans each row as a set key. Duplicate keys are intentionally
// deduplicated, including during Merge. Use map[K]struct{} to express a set.
func NewMapSetBinder[M ~map[K]struct{}, K comparable]() RowsBinder {
	return mapRowsBinder[M](true, nil, func(scanner RowsScanner) (func() (K, struct{}, error), *rowbind.Plan, error) {
		scan, err := prepareBindingScan(scanner, reflect.TypeFor[*K]())
		if err != nil {
			return nil, nil, err
		}

		args := []any{nil}
		if scan.ReusableMapValues() {
			var key K
			args[0] = &key
			return func() (K, struct{}, error) {
				var zero K
				key = zero
				err := scan.ScanCurrent(args...)
				return key, struct{}{}, err
			}, scan, nil
		}

		return func() (key K, value struct{}, err error) {
			args[0] = &key
			err = scan.ScanCurrent(args...)
			return
		}, scan, nil
	})
}

func mapRowsBinder[M ~map[K]V, K comparable, V any](set bool, configError error,
	prepare func(RowsScanner) (func() (K, V, error), *rowbind.Plan, error),
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

		return &mapRowsBinding[M, K, V]{
			pointer: pointer, prepare: prepare, capacity: options.capacity(), mode: options.Mode,
			duplicates: options.DuplicateKeys, set: set, checkKey: checkKey,
		}, nil
	})
}

type mapRowsBinding[M ~map[K]V, K comparable, V any] struct {
	pointer    *M
	staged     M
	prepare    func(RowsScanner) (func() (K, V, error), *rowbind.Plan, error)
	capacity   int
	mode       BindMode
	duplicates DuplicateKeyPolicy
	set        bool
	checkKey   bool
}

func (b *mapRowsBinding[M, K, V]) Commit() { *b.pointer = b.staged }

func (b *mapRowsBinding[M, K, V]) Scan(scanner RowsScanner) error {
	scan, plan, err := b.prepare(scanner)
	if err != nil {
		return err
	}
	defer rowbind.Release(plan)

	base := 0
	if b.mode == BindMerge {
		base = len(*b.pointer)
	}
	if base > int(^uint(0)>>1)-b.capacity {
		return errors.New("sqlx: map capacity overflow")
	}

	row := 0
	for scanner.Next() {
		row++
		if row == 1 {
			b.staged = make(M, base+b.capacity)
			if b.mode == BindMerge {
				maps.Copy(b.staged, *b.pointer)
			}
		}

		key, value, err := scan()
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
		if b.mode == BindMerge {
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
