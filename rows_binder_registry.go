// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"reflect"
	"sync"
	"sync/atomic"
	"time"
)

// RowsBinderRegistry dispatches by the exact type passed to [Rows.Bind],
// [Rows.Append] or [Rows.Merge]. An unregistered destination falls back to
// [SliceRowsBinder]. Its zero value is ready to use. [RowsBinderRegistry.Register],
// [RowsBinderRegistry.Get], [RowsBinderRegistry.Unregister] and [RowsBinderRegistry.Prepare]
// are concurrent safe; a registry must not be copied after first use. Registered
// binders must themselves support concurrent [RowsBinder.Prepare] calls.
type RowsBinderRegistry struct{ types sync.Map }

func NewRowsBinderRegistry() *RowsBinderRegistry { return &RowsBinderRegistry{} }

var defaultRowsBinderRegistry atomic.Value

func init() {
	SetDefaultRowsBinderRegistry(newDefaultRowsBinderRegistry())
}

// DefaultRowsBinderRegistry returns the shared default for collection binding.
// It is never nil.
//
// Common scalar slices, two-column maps and map sets are registered at initialization;
// [NewRegisteredOper] registers its model slice unless the exact destination is
// already registered. Use [RowsBinderRegistry.Register] for custom map semantics.
//
// DefaultRowsBinderRegistry and [SetDefaultRowsBinderRegistry] are safe for concurrent use.
func DefaultRowsBinderRegistry() *RowsBinderRegistry {
	return defaultRowsBinderRegistry.Load().(*RowsBinderRegistry)
}

// SetDefaultRowsBinderRegistry sets the registry returned by [DefaultRowsBinderRegistry].
// It panics if binder is nil, leaving the default unchanged. Changes affect
// subsequent bindings without an explicit binder, not bindings already prepared.
// Existing registrations stay with their registry and are not copied.
//
// SetDefaultRowsBinderRegistry and [DefaultRowsBinderRegistry] are safe for concurrent use.
func SetDefaultRowsBinderRegistry(binder *RowsBinderRegistry) {
	if binder == nil {
		panic("sqlx: nil default rows binder registry")
	}
	defaultRowsBinderRegistry.Store(binder)
}

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

func newDefaultRowsBinderRegistry() *RowsBinderRegistry {
	b := NewRowsBinderRegistry()
	for t, binder := range builtinSliceBinders {
		b.types.Store(t, binder)
	}

	// Map defaults are registrations only: removing one restores the slice
	// fallback, which does not infer map semantics.
	for t, binder := range map[reflect.Type]RowsBinder{
		reflect.TypeFor[*map[int]int]():    NewMapPairsBinder[map[int]int](),
		reflect.TypeFor[*map[int]int32]():  NewMapPairsBinder[map[int]int32](),
		reflect.TypeFor[*map[int]int64]():  NewMapPairsBinder[map[int]int64](),
		reflect.TypeFor[*map[int]string](): NewMapPairsBinder[map[int]string](),

		reflect.TypeFor[*map[int64]int]():    NewMapPairsBinder[map[int64]int](),
		reflect.TypeFor[*map[int64]int32]():  NewMapPairsBinder[map[int64]int32](),
		reflect.TypeFor[*map[int64]int64]():  NewMapPairsBinder[map[int64]int64](),
		reflect.TypeFor[*map[int64]string](): NewMapPairsBinder[map[int64]string](),

		reflect.TypeFor[*map[string]int]():    NewMapPairsBinder[map[string]int](),
		reflect.TypeFor[*map[string]int32]():  NewMapPairsBinder[map[string]int32](),
		reflect.TypeFor[*map[string]int64]():  NewMapPairsBinder[map[string]int64](),
		reflect.TypeFor[*map[string]string](): NewMapPairsBinder[map[string]string](),

		reflect.TypeFor[*map[int]struct{}]():    NewMapSetBinder[map[int]struct{}](),
		reflect.TypeFor[*map[int32]struct{}]():  NewMapSetBinder[map[int32]struct{}](),
		reflect.TypeFor[*map[int64]struct{}]():  NewMapSetBinder[map[int64]struct{}](),
		reflect.TypeFor[*map[string]struct{}](): NewMapSetBinder[map[string]struct{}](),
	} {
		b.types.Store(t, binder)
	}
	return b
}

// Get returns the explicitly registered binder, or nil. The slice fallback is
// not returned by [RowsBinderRegistry.Get]. For a destination &values, register
// [reflect.TypeOf](&values).
func (b *RowsBinderRegistry) Get(t reflect.Type) RowsBinder {
	if binder, ok := b.types.Load(t); ok {
		return binder.(RowsBinder)
	}
	return nil
}

// Register atomically replaces a registration and returns its previous binder.
// Nil types and nil (including typed nil) binders panic. A selected registration
// is authoritative: its preparation or scan error never triggers the fallback.
// Changes affect subsequent preparation, not already prepared bindings.
func (b *RowsBinderRegistry) Register(t reflect.Type, binder RowsBinder) (old RowsBinder) {
	if t == nil || nilBindingValue(binder) {
		panic("sqlx.RowsBinderRegistry: type and binder must not be nil")
	}
	if previous, loaded := b.types.Swap(t, binder); loaded {
		return previous.(RowsBinder)
	}
	return nil
}

// RegisterType is [RowsBinderRegistry.Register] with the exact destination type D.
// For example, using [NewMapIndexBinder]:
//
//	r.RegisterType[*map[int64]Model](NewMapIndexBinder[map[int64]Model](key))
func (b *RowsBinderRegistry) RegisterType[D any](binder RowsBinder) RowsBinder {
	return b.Register(reflect.TypeFor[D](), binder)
}

// Unregister removes an exact registration and returns it, or nil if absent.
// Subsequent bindings use [SliceRowsBinder], including its built-in scalar paths.
func (b *RowsBinderRegistry) Unregister(t reflect.Type) (old RowsBinder) {
	if previous, loaded := b.types.LoadAndDelete(t); loaded {
		return previous.(RowsBinder)
	}
	return nil
}

func (b *RowsBinderRegistry) Prepare(dst any, options BindOptions) (RowsBinding, error) {
	if err := options.validate(); err != nil {
		return nil, err
	}
	return resolveRowsBinder(b, reflect.TypeOf(dst)).Prepare(dst, options)
}

// Built-in routing is shared by registry preparation and [Rows.Bind].
// A registered user binder is returned intact, never bypassed.
func resolveRowsBinder(binder RowsBinder, t reflect.Type) RowsBinder {
	if registry, ok := binder.(*RowsBinderRegistry); ok {
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
func (b *RowsBinderRegistry) registerDefault(t reflect.Type, binder RowsBinder) {
	b.types.LoadOrStore(t, binder)
}
