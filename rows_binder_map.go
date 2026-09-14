// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"errors"
	"fmt"
	"maps"
	"reflect"

	"github.com/xgfone/go-sqlx/internal/rowbind"
)

// NewMapPairsBinder scans two positional columns as key and value. Duplicate
// keys fail by default. Only a non-nil pointer to M is accepted.
func NewMapPairsBinder[M ~map[K]V, K comparable, V any]() RowsBinder {
	return mapRowsBinder[M](false, nil, []reflect.Type{
		reflect.TypeFor[*K](),
		reflect.TypeFor[*V](),
	}, scanMapPairs[K, V])
}

func scanMapPairs[K comparable, V any](scan RowScanFunc, reusable rowbind.Reuse) func() (K, V, error) {
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
		func(scan RowScanFunc, reusable rowbind.Reuse) func() (K, V, error) {
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
		func(scan RowScanFunc, reusable rowbind.Reuse) func() (K, struct{}, error) {
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

		mapping, err := rowbind.Prepare(options.Columns, types, options.ScanOptions)
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

type scanMaker[K comparable, V any] func(RowScanFunc, rowbind.Reuse) func() (K, V, error)

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
	return b.mapping.WithScan(cursor, func(scan RowScanFunc, reusable rowbind.Reuse) error {
		return b.scanRows(cursor, scan, reusable)
	})
}

func (b *mapRowsBinding[M, K, V]) scanRows(scanner RowCursor, scan RowScanFunc, reusable rowbind.Reuse) error {
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
