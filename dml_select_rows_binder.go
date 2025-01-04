// Copyright 2025 xgfone
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package sqlx

import (
	"fmt"
	"reflect"
	"time"
)

// RowsBinder is an interface to bind the rows to dst that may be a map or slice.
type RowsBinder interface {
	BindRows(scanner RowScanner, dst any) error
}

// RowsBinderFunc is a function to bind the rows to dst that may be a map or slice.
type RowsBinderFunc func(scanner RowScanner, dst any) (err error)

// BindRows implements the interface RowsBinder.
func (f RowsBinderFunc) BindRows(scanner RowScanner, dst any) error { return f(scanner, dst) }

var (
	// DefaultRowsCap is the default capacity to allocate a map or slice for scanned rows.
	DefaultRowsCap = 16

	// DefaultSliceCap is the default mixed rows binder to bind the rows to a map or slice.
	//
	// It has registered some rows binders for specific-types, such as
	//   []struct
	//   []int, []int64, []string
	//   map[string]int, map[string]string
	//   map[string]bool, map[string]struct{}
	DefaultMixRowsBinder = NewMixRowsBinder()

	// CommonSliceRowsBinder is the common rows binder to bind the rows to a slice.
	//
	// Notice: it uses the reflect package for the default implementation.
	CommonSliceRowsBinder RowsBinder = RowsBinderFunc(commonSliceRowsBinder)
)

// MixRowsBinder is a mixed rows binder based on the reflected type.
type MixRowsBinder struct {
	types map[reflect.Type]RowsBinder
}

// NewMixRowsBinder returns a new MixRowsBinder.
func NewMixRowsBinder() *MixRowsBinder {
	return &MixRowsBinder{types: make(map[reflect.Type]RowsBinder, 16)}
}

// Register registers a rows binder for a specific type.
func (b *MixRowsBinder) Register(vtype reflect.Type, binder RowsBinder) (old RowsBinder) {
	if binder == nil {
		panic("sqlx.MixRowsBinder: binder-typed must not be nil")
	}

	old = b.types[vtype]
	b.types[vtype] = binder
	return
}

// BindRows implements the interface RowsBinder.
func (b *MixRowsBinder) BindRows(scanner RowScanner, dst any) (err error) {
	vtype := reflect.TypeOf(dst)
	if binder, ok := b.types[vtype]; ok {
		return binder.BindRows(scanner, dst)
	}

	if vtype.Kind() == reflect.Pointer && vtype.Elem().Kind() == reflect.Slice {
		return CommonSliceRowsBinder.BindRows(scanner, dst)
	}

	return fmt.Errorf("sqlx.MixRowsBinder.BindRows: unsupport the type %s for rows binder", vtype)
}

func init() {
	// []int, []uint, []int32, []uint32, []int64, []uint64, []string, []time.Time
	DefaultMixRowsBinder.Register(reflect.TypeFor[*[]int](), NewSliceRowsBinder[[]int]())
	DefaultMixRowsBinder.Register(reflect.TypeFor[*[]uint](), NewSliceRowsBinder[[]uint]())
	DefaultMixRowsBinder.Register(reflect.TypeFor[*[]int32](), NewSliceRowsBinder[[]int32]())
	DefaultMixRowsBinder.Register(reflect.TypeFor[*[]uint32](), NewSliceRowsBinder[[]uint32]())
	DefaultMixRowsBinder.Register(reflect.TypeFor[*[]int64](), NewSliceRowsBinder[[]int64]())
	DefaultMixRowsBinder.Register(reflect.TypeFor[*[]uint64](), NewSliceRowsBinder[[]uint64]())
	DefaultMixRowsBinder.Register(reflect.TypeFor[*[]string](), NewSliceRowsBinder[[]string]())
	DefaultMixRowsBinder.Register(reflect.TypeFor[*[]time.Time](), NewSliceRowsBinder[[]time.Time]())

	/// ------------------------------------- map[K]bool -------------------------------------- ///

	// map[int]bool
	DefaultMixRowsBinder.Register(reflect.TypeFor[map[int]bool](), NewMapRowsBinderForKey[map[int]bool](fixedvaluebooltrue[int]))
	DefaultMixRowsBinder.Register(reflect.TypeFor[*map[int]bool](), NewMapRowsBinderForKey[map[int]bool](fixedvaluebooltrue[int]))

	// map[int32]bool
	DefaultMixRowsBinder.Register(reflect.TypeFor[map[int32]bool](), NewMapRowsBinderForKey[map[int32]bool](fixedvaluebooltrue[int32]))
	DefaultMixRowsBinder.Register(reflect.TypeFor[*map[int32]bool](), NewMapRowsBinderForKey[map[int32]bool](fixedvaluebooltrue[int32]))

	// map[int64]bool
	DefaultMixRowsBinder.Register(reflect.TypeFor[map[int64]bool](), NewMapRowsBinderForKey[map[int64]bool](fixedvaluebooltrue[int64]))
	DefaultMixRowsBinder.Register(reflect.TypeFor[*map[int64]bool](), NewMapRowsBinderForKey[map[int64]bool](fixedvaluebooltrue[int64]))

	// map[string]bool
	DefaultMixRowsBinder.Register(reflect.TypeFor[map[string]bool](), NewMapRowsBinderForKey[map[string]bool](fixedvaluebooltrue[string]))
	DefaultMixRowsBinder.Register(reflect.TypeFor[*map[string]bool](), NewMapRowsBinderForKey[map[string]bool](fixedvaluebooltrue[string]))

	/// ----------------------------------- map[K]struct{} ------------------------------------ ///

	// map[int]struct{}
	DefaultMixRowsBinder.Register(reflect.TypeFor[map[int]struct{}](), NewMapRowsBinderForKey[map[int]struct{}](fixedvaluestructempty[int]))
	DefaultMixRowsBinder.Register(reflect.TypeFor[*map[int]struct{}](), NewMapRowsBinderForKey[map[int]struct{}](fixedvaluestructempty[int]))

	// map[int32]struct{}
	DefaultMixRowsBinder.Register(reflect.TypeFor[map[int32]struct{}](), NewMapRowsBinderForKey[map[int32]struct{}](fixedvaluestructempty[int32]))
	DefaultMixRowsBinder.Register(reflect.TypeFor[*map[int32]struct{}](), NewMapRowsBinderForKey[map[int32]struct{}](fixedvaluestructempty[int32]))

	// map[int64]struct{}
	DefaultMixRowsBinder.Register(reflect.TypeFor[map[int64]struct{}](), NewMapRowsBinderForKey[map[int64]struct{}](fixedvaluestructempty[int64]))
	DefaultMixRowsBinder.Register(reflect.TypeFor[*map[int64]struct{}](), NewMapRowsBinderForKey[map[int64]struct{}](fixedvaluestructempty[int64]))

	// map[string]struct{}
	DefaultMixRowsBinder.Register(reflect.TypeFor[map[string]struct{}](), NewMapRowsBinderForKey[map[string]struct{}](fixedvaluestructempty[string]))
	DefaultMixRowsBinder.Register(reflect.TypeFor[*map[string]struct{}](), NewMapRowsBinderForKey[map[string]struct{}](fixedvaluestructempty[string]))

	/// -------------------------------------- map[int]V -------------------------------------- ///

	// map[int]int
	DefaultMixRowsBinder.Register(reflect.TypeFor[map[int]int](), NewMapRowsBinderForKeyValue[map[int]int]())
	DefaultMixRowsBinder.Register(reflect.TypeFor[*map[int]int](), NewMapRowsBinderForKeyValue[map[int]int]())

	// map[int]int32
	DefaultMixRowsBinder.Register(reflect.TypeFor[map[int]int32](), NewMapRowsBinderForKeyValue[map[int]int32]())
	DefaultMixRowsBinder.Register(reflect.TypeFor[*map[int]int32](), NewMapRowsBinderForKeyValue[map[int]int32]())

	// map[int]int64
	DefaultMixRowsBinder.Register(reflect.TypeFor[map[int]int64](), NewMapRowsBinderForKeyValue[map[int]int64]())
	DefaultMixRowsBinder.Register(reflect.TypeFor[*map[int]int64](), NewMapRowsBinderForKeyValue[map[int]int64]())

	// map[int]string
	DefaultMixRowsBinder.Register(reflect.TypeFor[map[int]string](), NewMapRowsBinderForKeyValue[map[int]string]())
	DefaultMixRowsBinder.Register(reflect.TypeFor[*map[int]string](), NewMapRowsBinderForKeyValue[map[int]string]())

	/// ------------------------------------- map[int64]V ------------------------------------- ///

	// map[int64]int
	DefaultMixRowsBinder.Register(reflect.TypeFor[map[int64]int](), NewMapRowsBinderForKeyValue[map[int64]int]())
	DefaultMixRowsBinder.Register(reflect.TypeFor[*map[int64]int](), NewMapRowsBinderForKeyValue[map[int64]int]())

	// map[int64]int32
	DefaultMixRowsBinder.Register(reflect.TypeFor[map[int64]int32](), NewMapRowsBinderForKeyValue[map[int64]int32]())
	DefaultMixRowsBinder.Register(reflect.TypeFor[*map[int64]int32](), NewMapRowsBinderForKeyValue[map[int64]int32]())

	// map[int64]int64
	DefaultMixRowsBinder.Register(reflect.TypeFor[map[int64]int64](), NewMapRowsBinderForKeyValue[map[int64]int64]())
	DefaultMixRowsBinder.Register(reflect.TypeFor[*map[int64]int64](), NewMapRowsBinderForKeyValue[map[int64]int64]())

	// map[int64]string
	DefaultMixRowsBinder.Register(reflect.TypeFor[map[int64]string](), NewMapRowsBinderForKeyValue[map[int64]string]())
	DefaultMixRowsBinder.Register(reflect.TypeFor[*map[int64]string](), NewMapRowsBinderForKeyValue[map[int64]string]())

	/// ------------------------------------ map[string]V ------------------------------------- ///

	// map[string]int
	DefaultMixRowsBinder.Register(reflect.TypeFor[map[string]int](), NewMapRowsBinderForKeyValue[map[string]int]())
	DefaultMixRowsBinder.Register(reflect.TypeFor[*map[string]int](), NewMapRowsBinderForKeyValue[map[string]int]())

	// map[string]int32
	DefaultMixRowsBinder.Register(reflect.TypeFor[map[string]int32](), NewMapRowsBinderForKeyValue[map[string]int32]())
	DefaultMixRowsBinder.Register(reflect.TypeFor[*map[string]int32](), NewMapRowsBinderForKeyValue[map[string]int32]())

	// map[string]int64
	DefaultMixRowsBinder.Register(reflect.TypeFor[map[string]int64](), NewMapRowsBinderForKeyValue[map[string]int64]())
	DefaultMixRowsBinder.Register(reflect.TypeFor[*map[string]int64](), NewMapRowsBinderForKeyValue[map[string]int64]())

	// map[string]string
	DefaultMixRowsBinder.Register(reflect.TypeFor[map[string]string](), NewMapRowsBinderForKeyValue[map[string]string]())
	DefaultMixRowsBinder.Register(reflect.TypeFor[*map[string]string](), NewMapRowsBinderForKeyValue[map[string]string]())
}

func fixedvaluebooltrue[K comparable](K) bool        { return true }
func fixedvaluestructempty[K comparable](K) struct{} { return struct{}{} }

// NewMapRowsBinderForKey returns a rows binder which binds the rows as the map keys
// and extracts the map values from the keys.
func NewMapRowsBinderForKey[M ~map[K]V, K comparable, V any](valuef func(K) V) RowsBinder {
	return RowsBinderFunc(func(scanner RowScanner, dst any) (err error) {
		var m M
		switch v := dst.(type) {
		case M:
			if v == nil {
				panic("sqlx. NewMapRowsBinderForKey: map value must not be nil")
			}
			m = v

		case *M:
			if *v == nil {
				*v = make(M, getrowscap(scanner, DefaultRowsCap))
			}
			m = *v

		default:
			panic(fmt.Errorf("sqlx. NewMapRowsBinderForKey: unsupport the type %T", dst))
		}

		for scanner.Next() {
			var key K
			if err = scanner.Scan(&key); err != nil {
				return
			}
			m[key] = valuef(key)
		}

		return
	})
}

// NewMapRowsBinderForValue returns a rows binder which binds the rows as the map values
// and extracts the map keys from the values.
func NewMapRowsBinderForValue[M ~map[K]V, K comparable, V any](keyf func(V) K) RowsBinder {
	return RowsBinderFunc(func(scanner RowScanner, dst any) (err error) {
		var m M
		switch v := dst.(type) {
		case M:
			if v == nil {
				panic("sqlx.NewMapRowsBinderForValue: map value must not be nil")
			}
			m = v

		case *M:
			if *v == nil {
				*v = make(M, getrowscap(scanner, DefaultRowsCap))
			}
			m = *v

		default:
			panic(fmt.Errorf("sqlx.NewMapRowsBinderForValue: unsupport the type %T", dst))
		}

		for scanner.Next() {
			var value V
			if err = scanner.Scan(&value); err != nil {
				return
			}
			m[keyf(value)] = value
		}

		return
	})
}

// NewMapRowsBinderForKeyValue returns a rows binder which binds the rows as the map keys and values.
//
// Notice: each row must have two columns as key and value from front to back.
func NewMapRowsBinderForKeyValue[M ~map[K]V, K comparable, V any]() RowsBinder {
	return RowsBinderFunc(func(scanner RowScanner, dst any) (err error) {
		var m M
		switch v := dst.(type) {
		case M:
			if v == nil {
				panic("sqlx. NewMapRowsBinderForKeyValue: map value must not be nil")
			}
			m = v

		case *M:
			if *v == nil {
				*v = make(M, getrowscap(scanner, DefaultRowsCap))
			}
			m = *v

		default:
			panic(fmt.Errorf("sqlx. NewMapRowsBinderForKeyValue: unsupport the type %T", dst))
		}

		for scanner.Next() {
			var key K
			var value V
			if err = scanner.Scan(&key, &value); err != nil {
				return
			}
			m[key] = value
		}

		return
	})
}

// NewSliceRowsBinder returns a rows binder which binds the rows as the slice.
//
// It does not use the reflect package.
func NewSliceRowsBinder[S ~[]T, T any]() RowsBinder {
	return RowsBinderFunc(func(scanner RowScanner, dst any) (err error) {
		dstps, ok := dst.(*[]T)
		if !ok {
			panic(fmt.Errorf("sqlx. NewSliceRowsBinder: expect type %T, but got %T", (*S)(nil), dst))
		}

		dsts := *dstps
		if cap(dsts) == 0 {
			dsts = make(S, 0, getrowscap(scanner, DefaultRowsCap))
		}

		for scanner.Next() {
			var value T
			if err := scanner.Scan(&value); err != nil {
				return err
			}
			dsts = append(dsts, value)
		}

		*dstps = dsts
		return
	})
}

func commonSliceRowsBinder(scanner RowScanner, dst any) (err error) {
	oldvf := reflect.ValueOf(dst)
	if oldvf.Kind() != reflect.Ptr {
		panic("sqlx.CommonSliceRowsBinder: the value must be a pointer to a slice")
	}

	vf := oldvf.Elem()
	if vf.Kind() != reflect.Slice {
		panic("sqlx.CommonSliceRowsBinder: the value must be a pointer to a slice")
	}

	vt := vf.Type()
	et := vt.Elem()
	if vf.Cap() == 0 {
		vf.Set(reflect.MakeSlice(vt, 0, getrowscap(scanner, DefaultRowsCap)))
	}

	for scanner.Next() {
		e := reflect.New(et)
		if err := scanner.Scan(e.Interface()); err != nil {
			return err
		}
		vf = reflect.Append(vf, e.Elem())
	}

	oldvf.Elem().Set(vf)
	return
}

// NewDegradedSliceRowsBinder returns a rows binder which prefers to try to
// bind *[]T to the rows, or use the degraded rows binder to bind the rows.
func NewDegradedSliceRowsBinder[S ~[]T, T any](degraded RowsBinder) RowsBinder {
	binder := NewSliceRowsBinder[S]()
	return RowsBinderFunc(func(scanner RowScanner, dst any) error {
		if dstps, ok := dst.(*[]T); ok {
			return binder.BindRows(scanner, dstps)
		}
		return degraded.BindRows(scanner, dst)
	})
}
