// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"database/sql/driver"
	"reflect"
	"sync"
	"testing"

	"github.com/xgfone/go-sqlx/internal/rowbind"
)

func TestProjectionSharesBindingMetadata(t *testing.T) {
	type child struct {
		ID int64 `sql:"id"`
	}
	type record struct {
		Child child  `sql:"child"`
		Name  string `sql:"name,omitempty"`
	}

	typ := reflect.TypeFor[record]()
	projectionCache.Delete(typ) // Also exercise lazy creation with go test -count.

	var got record
	err := ScanColumnsToStruct(func(dst ...any) error {
		*dst[0].(*int64), *dst[1].(*string) = 7, "name"
		return nil
	}, []string{"child_id", "name"}, &got)
	if err != nil || got.Child.ID != 7 || got.Name != "name" {
		t.Fatal(got, err)
	}
	if _, ok := projectionCache.Load(typ); ok {
		t.Fatal("binding eagerly allocated SQL expression storage")
	}

	meta, err := rowbind.Describe(typ)
	if err != nil {
		t.Fatal(err)
	}

	const count = 16
	results := make(chan *modelProjection, count)
	var wg sync.WaitGroup
	for range count {
		wg.Go(func() {
			// All pointer depths use the same normalized type and projection.
			p, err := projectionFor(reflect.PointerTo(typ))
			if err != nil {
				t.Error(err)
				return
			}
			results <- p
		})
	}
	wg.Wait()

	close(results)
	first := <-results
	if first == nil || first.meta != meta || len(first.columns) != 2 ||
		first.columns[0].Column != "child_id" || first.columns[1].Column != "name" {
		t.Fatal("projection did not reuse binding metadata", first)
	}

	for p := range results {
		if p != first {
			t.Fatal("concurrent compilation published different projections")
		}
	}

	allocs := testing.AllocsPerRun(100, func() {
		p, err := projectionFor(typ)
		if err != nil || p != first {
			panic("projection cache changed")
		}
	})
	if allocs != 0 {
		t.Fatalf("warm projection lookup allocated: %v", allocs)
	}
}

func TestStructFieldConsistencyAndPointers(t *testing.T) {
	type Child struct {
		Count int `sql:"count"`
	}

	type Model struct {
		ID       int    `sql:"id,omitempty"`
		Child    *Child `sql:"child"`
		Optional *int   `sql:"optional"`
		hidden   string //nolint:unused
	}

	m := Model{ID: 2}
	checkSQL(t, Select().SelectStruct(m, "a").FromAlias("t", "a"),
		"SELECT `a`.`id`, `a`.`child_count`, `a`.`optional` FROM `t` AS `a`")
	checkSQL(t, Insert().Into("t").Struct(m),
		"INSERT INTO `t` (`id`, `child_count`, `optional`) VALUES (?, ?, ?)",
		2, nil, nil)

	e := ScanColumnsToStruct(func(vs ...any) error {
		*vs[0].(*int) = 5
		*vs[1].(**int) = nil
		return nil
	}, []string{"child_count", "optional"}, &m)
	if e != nil || m.Child == nil || m.Child.Count != 5 {
		t.Fatalf("%+v %v", m, e)
	}

	type Omit struct {
		A int `sql:"a,omitempty"`
		B int `sql:"b,omitempty"`
	}

	checkSQL(t, Insert().Into("t").Structs([]Omit{{A: 1}, {B: 2}}),
		"INSERT INTO `t` (`a`, `b`) VALUES (?, DEFAULT), (DEFAULT, ?)", 1, 2)
	checkSQL(t, Insert().Into("t").Columns("b", "a").Structs([]*Omit{{A: 1}, {B: 2}}),
		"INSERT INTO `t` (`b`, `a`) VALUES (?, ?), (?, ?)", 0, 1, 2, 0)

	type Duplicate struct {
		A int `sql:"x"`
		B int `sql:"x"`
	}

	checkBuildError(t, Select().SelectStruct(Duplicate{}))
	checkBuildError(t, Insert().Into("t").Struct((*Model)(nil)))

	if e := ScanColumnsToStruct(func(...any) error { return nil }, []string{"id"}, (*Model)(nil)); e == nil {
		t.Fatal("nil destination accepted")
	}
	if e := ScanColumnsToStruct(func(...any) error { return nil }, []string{"id", "id"}, &m); e == nil {
		t.Fatal("ambiguous columns accepted")
	}

	type Recursive struct{ Next *Recursive }
	checkBuildError(t, Select().SelectStruct(Recursive{}))
}

type dynamicColumns struct{ column string }

func (v dynamicColumns) Columns(q string) []Namer { return []Namer{{Name: v.column}} }

type pointerValue struct{ Number int }

func (v *pointerValue) Value() (driver.Value, error) { return int64(v.Number), nil }

func TestStructProvidersAndPointerValuers(t *testing.T) {
	type Model struct {
		V pointerValue `sql:"v"`
	}

	checkSQL(t, Select().SelectStruct(dynamicColumns{"first"}).From("t"), "SELECT `first` FROM `t`")
	checkSQL(t, Select().SelectStruct(dynamicColumns{"second"}).From("t"), "SELECT `second` FROM `t`")
	checkSQL(t, Select().SelectStruct(Model{}).From("t"), "SELECT `v` FROM `t`")

	q, args, e := Insert().Into("t").Struct(Model{V: pointerValue{8}}).Build()
	if e != nil || q != "INSERT INTO `t` (`v`) VALUES (?)" {
		t.Fatalf("%s %v", q, e)
	}

	v, e := args[0].(driver.Valuer).Value()
	if e != nil || v != int64(8) {
		t.Fatalf("%v %v", v, e)
	}
}
