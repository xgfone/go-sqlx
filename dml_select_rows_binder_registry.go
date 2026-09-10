// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"reflect"
	"sync"
	"time"
)

// MixRowsBinder dispatches by the exact type passed to Rows.Bind, Append or
// Merge. An unregistered destination falls back to SliceRowsBinder. Its zero
// value is ready to use. Register, Get, Unregister and Prepare are concurrent
// safe; a registry must not be copied after first use. Registered binders must
// themselves support concurrent Prepare calls.
type MixRowsBinder struct{ types sync.Map }

func NewMixRowsBinder() *MixRowsBinder { return &MixRowsBinder{} }

// DefaultMixRowsBinder is the shared default for collection binding. Common
// scalar slices are registered at initialization; NewOper also registers its
// model slice unless that exact destination type already has a registration.
// Register custom map semantics explicitly. Configure this variable before
// use; use the registry methods for concurrent changes rather than reassigning it.
var DefaultMixRowsBinder = newDefaultMixRowsBinder()

var builtinSliceBinders = map[reflect.Type]RowsBinder{
	reflect.TypeFor[*[]int]():           NewSliceRowsBinder[[]int](),
	reflect.TypeFor[*[]int32]():         NewSliceRowsBinder[[]int32](),
	reflect.TypeFor[*[]int64]():         NewSliceRowsBinder[[]int64](),
	reflect.TypeFor[*[]uint]():          NewSliceRowsBinder[[]uint](),
	reflect.TypeFor[*[]uint32]():        NewSliceRowsBinder[[]uint32](),
	reflect.TypeFor[*[]uint64]():        NewSliceRowsBinder[[]uint64](),
	reflect.TypeFor[*[]float64]():       NewSliceRowsBinder[[]float64](),
	reflect.TypeFor[*[]bool]():          NewSliceRowsBinder[[]bool](),
	reflect.TypeFor[*[]string]():        NewSliceRowsBinder[[]string](),
	reflect.TypeFor[*[]time.Time]():     NewSliceRowsBinder[[]time.Time](),
	reflect.TypeFor[*[]time.Duration](): NewSliceRowsBinder[[]time.Duration](),
	reflect.TypeFor[*[][]byte]():        NewSliceRowsBinder[[][]byte](),
	reflect.TypeFor[*[]any]():           NewSliceRowsBinder[[]any](),
}

func newDefaultMixRowsBinder() *MixRowsBinder {
	b := NewMixRowsBinder()
	for t, binder := range builtinSliceBinders {
		b.types.Store(t, binder)
	}
	return b
}

// Get returns the explicitly registered binder, or nil. The slice fallback is
// not returned by Get. For a destination &values, register reflect.TypeOf(&values).
func (b *MixRowsBinder) Get(t reflect.Type) RowsBinder {
	if binder, ok := b.types.Load(t); ok {
		return binder.(RowsBinder)
	}
	return nil
}

// Register atomically replaces a registration and returns its previous binder.
// Nil types and nil (including typed nil) binders panic. A selected registration
// is authoritative: its preparation or scan error never triggers the fallback.
// Changes affect subsequent preparation, not already prepared bindings.
func (b *MixRowsBinder) Register(t reflect.Type, binder RowsBinder) (old RowsBinder) {
	if t == nil || nilBindingValue(binder) {
		panic("sqlx.MixRowsBinder: type and binder must not be nil")
	}
	if previous, loaded := b.types.Swap(t, binder); loaded {
		return previous.(RowsBinder)
	}
	return nil
}

// RegisterType is Register with the exact destination type D. For example,
// RegisterType[*map[int64]Model](NewMapIndexBinder[map[int64]Model](key)).
func (b *MixRowsBinder) RegisterType[D any](binder RowsBinder) RowsBinder {
	return b.Register(reflect.TypeFor[D](), binder)
}

// Unregister removes an exact registration and returns it, or nil if absent.
// Subsequent bindings use SliceRowsBinder, including its built-in scalar paths.
func (b *MixRowsBinder) Unregister(t reflect.Type) (old RowsBinder) {
	if previous, loaded := b.types.LoadAndDelete(t); loaded {
		return previous.(RowsBinder)
	}
	return nil
}

func (b *MixRowsBinder) Prepare(dst any, options BindOptions) (RowsBinding, error) {
	if err := options.validate(); err != nil {
		return nil, err
	}
	return resolveRowsBinder(b, reflect.TypeOf(dst)).Prepare(dst, options)
}

// Built-in routing is shared by registry preparation and Rows.Bind.
// A registered user binder is returned intact, never bypassed.
func resolveRowsBinder(binder RowsBinder, t reflect.Type) RowsBinder {
	if registry, ok := binder.(*MixRowsBinder); ok {
		if selected := registry.Get(t); selected != nil {
			return selected
		}
		binder = SliceRowsBinder{}
	}
	if _, ok := binder.(SliceRowsBinder); ok {
		if selected := builtinSliceBinders[t]; selected != nil {
			return selected
		}
	}
	return binder
}

// Only constructors use this path; explicit user registrations always win.
func (b *MixRowsBinder) registerDefault(t reflect.Type, binder RowsBinder) {
	b.types.LoadOrStore(t, binder)
}
