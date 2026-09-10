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
	if _, err := registry.Prepare(&slice, BindOptions{}); err != nil {
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
	err := rows.WithBinder(registry).Bind(&original)
	if !errors.Is(err, rejected) || original[0] != 9 || fixture.next.Load() != 0 || fixture.closed.Load() != 1 {
		t.Fatal("registered error was retried or cursor leaked", original, err)
	}

	var got []rejectingScanValue
	registry.RegisterType[*[]rejectingScanValue](NewSliceRowsBinder[[]rejectingScanValue]())
	rows, fixture = bindTestRows(t, "bad", int64(8))
	err = rows.WithBinder(registry).Bind(&got)
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
		Capacity: 3,
		Scan:     ScanOptions{DurationUnit: time.Hour},
	})

	o := NewOper[model]("t").WithDB(db)
	if DefaultMixRowsBinder.Get(dstType) != global {
		t.Fatal("NewOper overwrote a user registration")
	}

	ctx := context.Background()
	queries := []struct {
		rows func() Rows
		want *countingRowsBinder
	}{
		{func() Rows { return db.QueryRowsContext(ctx, "q") }, global},
		{func() Rows { return o.SelectStruct().QueryRowsContext(ctx) }, global},
		{func() Rows { return db.WithBinder(local).QueryRowsContext(ctx, "q") }, local},
		{func() Rows { return o.WithBinder(local).SelectStruct().QueryRowsContext(ctx) }, local},
		{func() Rows { return o.SelectStruct().QueryRowsContext(ctx).WithBinder(local) }, local},
		{func() Rows { return o.WithBinder(local).SelectStruct().QueryRowsContext(ctx).WithBinder(nil) }, global},
		{func() Rows { return o.WithBinder(local).WithBinder(nil).SelectStruct().QueryRowsContext(ctx) }, global},
		{func() Rows { return db.WithBinder(local).WithBinder(nil).QueryRowsContext(ctx, "q") }, global},
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

func TestNewOperRegistersModelSliceOnce(t *testing.T) {
	type model struct{ ID int64 }
	dstType := reflect.TypeFor[*[]model]()
	t.Cleanup(func() { DefaultMixRowsBinder.Unregister(dstType) })

	NewOper[model]("first")
	if _, ok := DefaultMixRowsBinder.Get(dstType).(typedSliceRowsBinder[[]model, model]); !ok {
		t.Fatal("missing typed model binder")
	}

	NewOper[model]("second")
	if DefaultMixRowsBinder.Get(reflect.TypeFor[*[]int64]()) == nil {
		t.Fatal("missing built-in scalar registration")
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
	binding, err := registry.Prepare(&got, BindOptions{})
	if err != nil {
		t.Fatal(err)
	}

	replaced := errors.New("replacement binder")
	registry.RegisterType[*[]int64](RowsBinderFunc(func(any, BindOptions) (RowsBinding, error) {
		return nil, replaced
	}))

	rows, _ := bindTestRows(t, int64(7))
	defer rows.Close() //nolint:errcheck

	if err := binding.Scan(rows); err != nil {
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
