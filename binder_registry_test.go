// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"context"
	"database/sql/driver"
	"errors"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type countingRowsBinder struct {
	delegate RowsBinder
	calls    atomic.Int64
}

func TestDefaultMapRegistrations(t *testing.T) {
	for _, want := range []any{
		map[int]int{7: 9},
		map[int]int32{7: 9},
		map[int]int64{7: 9},
		map[int]string{7: "9"},
		map[int64]int{7: 9},
		map[int64]int32{7: 9},
		map[int64]int64{7: 9},
		map[int64]string{7: "9"},
		map[string]int{"7": 9},
		map[string]int32{"7": 9},
		map[string]int64{"7": 9},
		map[string]string{"7": "9"},
		map[int]struct{}{7: {}},
		map[int32]struct{}{7: {}},
		map[int64]struct{}{7: {}},
		map[string]struct{}{"7": {}},
	} {
		t.Run(reflect.TypeOf(want).String(), func(t *testing.T) {
			f := &bindFixture{
				columns: []string{"key", "value"},
				values:  [][]driver.Value{{"7", "9"}},
			}
			if reflect.TypeOf(want).Elem() == reflect.TypeFor[struct{}]() {
				// Sets must deduplicate repeated rows under the default policy.
				f.columns, f.values = []string{"key"}, [][]driver.Value{{"7"}, {"7"}}
			}

			dst := reflect.New(reflect.TypeOf(want)) // Pointer to an unallocated map.
			err := bindTestDB(t, f).QueryRowsContext(context.Background(), "q").Bind(dst.Interface())
			if err != nil || !reflect.DeepEqual(dst.Elem().Interface(), want) {
				t.Fatalf("got %v, error %v; want %v", dst.Elem().Interface(), err, want)
			}

			for _, value := range []any{reflect.Zero(reflect.TypeOf(want)).Interface(), want} {
				_, err := DefaultMixRowsBinder.Prepare(value, BindOptions{})
				if !IsUnsupportedTypeError(err) {
					t.Fatalf("map by value must be unsupported: %v", err)
				}
			}
		})
	}
}

func TestDefaultMapDestinationAndCommit(t *testing.T) {
	query := func(values ...[]driver.Value) *Rows {
		return bindTestDB(t, &bindFixture{
			columns: []string{"key", "value"},
			values:  values,
		}).QueryRowsContext(context.Background(), "q")
	}

	got := map[string]int64{"old": 1}
	alias := got
	if err := query([]driver.Value{"new", int64(2)}).Bind(&got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, map[string]int64{"new": 2}) ||
		!reflect.DeepEqual(alias, map[string]int64{"old": 1}) {
		t.Fatal("replacement changed old storage", got, alias)
	}
	if err := query([]driver.Value{"added", int64(3)}).Merge(&got); err != nil {
		t.Fatal(err)
	}

	want := map[string]int64{"new": 2, "added": 3}
	if !reflect.DeepEqual(got, want) {
		t.Fatal("merge lost entries", got)
	}

	for _, merge := range []bool{false, true} {
		rows := query([]driver.Value{"new", int64(4)}, []driver.Value{"new", int64(5)})
		var err error
		if merge {
			err = rows.Merge(&got)
		} else {
			err = rows.Bind(&got)
		}

		var duplicate *DuplicateKeyError
		if !errors.As(err, &duplicate) || !reflect.DeepEqual(got, want) {
			t.Fatal("duplicate keys must fail without publishing", got, err)
		}
	}

	var nilMap map[string]int64
	var nilPointer *map[string]int64
	for _, dst := range []any{nilMap, make(map[string]int64), nilPointer} {
		f := &bindFixture{
			columns: []string{"key", "value"},
			values:  [][]driver.Value{{"k", int64(1)}},
		}
		err := bindTestDB(t, f).QueryRowsContext(context.Background(), "q").Bind(dst)
		if err == nil || f.next.Load() != 0 || f.closed.Load() != 1 {
			t.Fatal("invalid map destination must fail before iteration and close rows", err)
		}
	}
}

func TestDefaultMapRegistrationBoundaries(t *testing.T) {
	registry := newDefaultMixRowsBinder()
	type namedMap map[string]int64
	for _, dst := range []any{
		new(map[string]bool),
		new(namedMap),
		new(map[int64]struct{ ID int64 }),
	} {
		_, err := registry.Prepare(dst, BindOptions{})
		if !IsUnsupportedTypeError(err) {
			t.Fatal("unexpected implicit map semantics", err)
		}
	}

	var got map[string]int64
	_, err := NewMixRowsBinder().Prepare(&got, BindOptions{})
	if !IsUnsupportedTypeError(err) {
		t.Fatal("independent registry must start without map defaults", err)
	}

	rejected := errors.New("custom map binder")
	binder := func(any, BindOptions) (RowsBinding, error) { return nil, rejected }
	if registry.RegisterType[*map[string]int64](RowsBinderFunc(binder)) == nil {
		t.Fatal("missing default registration")
	}

	if _, err := registry.Prepare(&got, BindOptions{}); !errors.Is(err, rejected) {
		t.Fatal("custom registration must win", err)
	}

	registry.Unregister(reflect.TypeFor[*map[string]int64]())
	if _, err := registry.Prepare(&got, BindOptions{}); !IsUnsupportedTypeError(err) {
		t.Fatal("unregistered map must not fall back to a built-in binder", err)
	}
}

func (b *countingRowsBinder) Prepare(dst any, options BindOptions) (RowsBinding, error) {
	b.calls.Add(1)
	return b.delegate.Prepare(dst, options)
}

func TestMixRowsBinderRegistration(t *testing.T) {
	type amounts map[string]int64
	var registry MixRowsBinder // The zero value also has the slice fallback.

	binder := NewMapPairsBinder[amounts]()
	dstType := reflect.TypeFor[*amounts]()
	if registry.Get(dstType) != nil || registry.RegisterType[*amounts](binder) != nil {
		t.Fatal("unexpected previous registration")
	}
	if registry.Get(dstType) == nil || registry.Get(reflect.TypeFor[amounts]()) != nil {
		t.Fatal("registration must match the exact destination type")
	}

	var got amounts
	f := &bindFixture{columns: []string{"key", "value"}, values: [][]driver.Value{{"a", int64(3)}}}
	err := bindTestDB(t, f).WithBinder(&registry).QueryRowsContext(context.Background(), "q").Bind(&got)
	if err != nil || got["a"] != 3 {
		t.Fatal(got, err)
	}
	if registry.Unregister(dstType) == nil || registry.Unregister(dstType) != nil {
		t.Fatal("incorrect removal result")
	}
	if _, err := registry.Prepare(&got, BindOptions{}); !IsUnsupportedTypeError(err) {
		t.Fatal("map semantics must be registered", err)
	}

	var slice []int64
	if _, err := registry.Prepare(&slice, BindOptions{Columns: []string{"value"}}); err != nil {
		t.Fatal("missing slice fallback", err)
	}
	if _, err := registry.Prepare(nil, BindOptions{}); !IsUnsupportedTypeError(err) {
		t.Fatal("nil destination must return a type error", err)
	}

	for _, register := range []func(){
		func() { registry.Register(nil, binder) },
		func() { registry.Register(dstType, nil) },
		func() { registry.Register(dstType, (*countingRowsBinder)(nil)) },
		func() { registry.Register(dstType, RowsBinderFunc(nil)) },
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Error("invalid registration did not panic")
				}
			}()
			register()
		}()
	}
}

func TestMixRowsBinderSelectedErrorsDoNotFallBack(t *testing.T) {
	registry := NewMixRowsBinder()
	rejected := UnsupportedTypeError{Name: "custom", Type: "[]int64"}
	registry.RegisterType[*[]int64](RowsBinderFunc(func(any, BindOptions) (RowsBinding, error) {
		return nil, rejected
	}))

	original := []int64{9}
	rows, fixture := bindTestRows(t, int64(7))
	err := rows.SetBinder(registry).Bind(&original)
	if !errors.Is(err, rejected) || original[0] != 9 || fixture.next.Load() != 0 || fixture.closed.Load() != 1 {
		t.Fatal("registered error was retried or cursor leaked", original, err)
	}

	var got []rejectingScanValue
	registry.RegisterType[*[]rejectingScanValue](NewSliceRowsBinder[[]rejectingScanValue]())
	rows, fixture = bindTestRows(t, "bad", int64(8))
	err = rows.SetBinder(registry).Bind(&got)
	if err == nil || got != nil || fixture.next.Load() != 1 || fixture.closed.Load() != 1 {
		t.Fatal("scan failure was retried or committed", got, err)
	}
}

func TestDefaultRegistryAndBinderOverrides(t *testing.T) {
	type model struct {
		Value time.Duration `sql:"value"`
	}

	dstType := reflect.TypeFor[*[]model]()
	newBinder := func() *countingRowsBinder {
		return &countingRowsBinder{delegate: NewSliceRowsBinder[[]model]()}
	}

	global, local, late := newBinder(), newBinder(), newBinder()
	previous := DefaultMixRowsBinder.RegisterType[*[]model](global)
	t.Cleanup(func() {
		if previous == nil {
			DefaultMixRowsBinder.Unregister(dstType)
		} else {
			DefaultMixRowsBinder.Register(dstType, previous)
		}
	})

	db := bindTestDB(t, &bindFixture{
		columns: []string{"value"},
		values:  [][]driver.Value{{int64(2)}},
	}).WithBindConfig(BindConfig{
		Capacity:    3,
		ScanOptions: ScanOptions{DurationUnit: time.Hour},
	})

	o := NewRegisteredOper[model]("t").WithDB(db)
	if DefaultMixRowsBinder.Get(dstType) != global {
		t.Fatal("NewRegisteredOper overwrote a user registration")
	}

	ctx := context.Background()
	queries := []struct {
		rows func() *Rows
		want *countingRowsBinder
	}{
		{func() *Rows { return db.QueryRowsContext(ctx, "q") }, global},
		{func() *Rows { return o.SelectStruct().QueryRowsContext(ctx) }, global},
		{func() *Rows { return db.WithBinder(local).QueryRowsContext(ctx, "q") }, local},
		{func() *Rows { return o.WithBinder(local).SelectStruct().QueryRowsContext(ctx) }, local},
		{func() *Rows { return o.SelectStruct().QueryRowsContext(ctx).SetBinder(local) }, local},
		{func() *Rows { return o.WithBinder(local).SelectStruct().QueryRowsContext(ctx).SetBinder(nil) }, global},
		{func() *Rows { return o.WithBinder(local).WithBinder(nil).SelectStruct().QueryRowsContext(ctx) }, global},
		{func() *Rows { return db.WithBinder(local).WithBinder(nil).QueryRowsContext(ctx, "q") }, global},
	}

	for i, query := range queries {
		var got []model
		before := query.want.calls.Load()
		err := query.rows().Bind(&got)
		if err != nil || len(got) != 1 || cap(got) != 3 || got[0].Value != 2*time.Hour {
			t.Fatalf("query %d lost inherited options: %+v, %v", i, got, err)
		}
		if query.want.calls.Load() != before+1 {
			t.Fatalf("query %d used the wrong binder", i)
		}
	}

	var got []model
	DefaultMixRowsBinder.RegisterType[*[]model](late)
	err := o.SelectStruct().QueryRowsContext(ctx).Bind(&got)
	if err != nil || late.calls.Load() != 1 {
		t.Fatal("existing Oper did not observe subsequent registration", err)
	}
}

func TestOperConstructorRegistration(t *testing.T) {
	type model struct {
		ID int64 `sql:"id"`
	}

	dstType := reflect.TypeFor[*[]model]()
	t.Cleanup(func() { DefaultMixRowsBinder.Unregister(dstType) })
	db := bindTestDB(t, &bindFixture{
		columns: []string{"id"},
		values:  [][]driver.Value{{int64(7)}},
	})
	table := db.NewTable("models")
	constructors := []struct {
		name     string
		create   func() Oper[model]
		register bool
	}{
		{"NewOper", func() Oper[model] { return NewOper[model](table.Name).WithDB(db) }, false},
		{"Table.NewOper", func() Oper[model] { return table.NewOper[model]() }, false},
		{"NewRegisteredOper", func() Oper[model] { return NewRegisteredOper[model](table.Name).WithDB(db) }, true},
		{"Table.NewRegisteredOper", func() Oper[model] { return table.NewRegisteredOper[model]() }, true},
	}
	for _, constructor := range constructors {
		t.Run(constructor.name, func(t *testing.T) {
			DefaultMixRowsBinder.Unregister(dstType)
			o := constructor.create()
			if o.Table != table || o.GetDB() != db {
				t.Fatal("constructor lost table or database")
			}
			if o.SoftCondition == nil || o.DeletedCondition == nil || o.SoftDeleteUpdater == nil {
				t.Fatal("constructor lost soft-delete defaults")
			}
			if binder := DefaultMixRowsBinder.Get(dstType); constructor.register {
				if _, ok := binder.(typedSliceRowsBinder[[]model, model]); !ok {
					t.Fatal("missing typed model binder")
				}
				constructor.create()
				if DefaultMixRowsBinder.Get(dstType) != binder {
					t.Fatal("constructor replaced existing binder")
				}
			} else if binder != nil {
				t.Fatal("constructor registered a binder")
			}

			got, err := o.Gets(context.Background(), PageSize(1, 20))
			if err != nil || len(got) != 1 || got[0].ID != 7 {
				t.Fatal("model query failed", got, err)
			}
			if !constructor.register && DefaultMixRowsBinder.Get(dstType) != nil {
				t.Fatal("query registered a binder")
			}

			custom := &countingRowsBinder{delegate: NewSliceRowsBinder[[]model]()}
			DefaultMixRowsBinder.Register(dstType, custom)
			constructor.create()
			if DefaultMixRowsBinder.Get(dstType) != custom {
				t.Fatal("constructor overwrote a user registration")
			}
		})
	}
}

func TestMixRowsBinderConcurrent(t *testing.T) {
	registry := NewMixRowsBinder()
	dstType := reflect.TypeFor[*[]int64]()

	first := &countingRowsBinder{delegate: NewSliceRowsBinder[[]int64]()}
	second := &countingRowsBinder{delegate: NewSliceRowsBinder[[]int64]()}

	db := bindTestDB(t, &bindFixture{
		columns: []string{"value"},
		values:  [][]driver.Value{{int64(1)}, {int64(2)}},
	}).WithBinder(registry)

	var wg sync.WaitGroup
	for worker := range 8 {
		wg.Go(func() {
			for range 100 {
				if worker%2 == 0 {
					registry.Register(dstType, first)
				} else {
					registry.Register(dstType, second)
				}

				var got []int64
				err := db.QueryRowsContext(context.Background(), "q").Bind(&got)
				if err != nil || len(got) != 2 || got[0] != 1 || got[1] != 2 {
					t.Errorf("concurrent binding: %v, %v", got, err)
					return
				}

				registry.Get(dstType)
				registry.Unregister(dstType)
			}
		})
	}
	wg.Wait()
}

func TestRegistryReplacementDoesNotChangePreparedBinding(t *testing.T) {
	registry := NewMixRowsBinder()
	registry.RegisterType[*[]int64](NewSliceRowsBinder[[]int64]())

	var got []int64
	binding, err := registry.Prepare(&got, BindOptions{Columns: []string{"value"}})
	if err != nil {
		t.Fatal(err)
	}

	replaced := errors.New("replacement binder")
	registry.RegisterType[*[]int64](RowsBinderFunc(func(any, BindOptions) (RowsBinding, error) {
		return nil, replaced
	}))

	rows, _ := bindTestRows(t, int64(7))
	defer rows.Close() //nolint:errcheck

	if err := binding.Scan(rows.rows); err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Fatal("preparation/scan published before commit")
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}

	binding.Commit()
	if len(got) != 1 || got[0] != 7 {
		t.Fatal("existing binding changed after registration", got)
	}
	if _, err := registry.Prepare(&got, BindOptions{}); !errors.Is(err, replaced) {
		t.Fatal("new preparation did not use replacement", err)
	}
}
