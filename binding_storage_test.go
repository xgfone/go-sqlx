// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"context"
	"database/sql/driver"
	"fmt"
	"reflect"
	"testing"
)

type selfRetainingScanner struct {
	Value int64
	Self  *selfRetainingScanner
}

func (v *selfRetainingScanner) Scan(src any) error {
	v.Value, v.Self = src.(int64), v
	return nil
}

func TestMapScannerDestinationsRemainIndependent(t *testing.T) {
	for _, pairs := range []bool{false, true} {
		var binder RowsBinder
		var rows Rows
		if pairs {
			binder = NewMapPairsBinder[map[int64]selfRetainingScanner]()
			rows = bindTestDB(t, &bindFixture{
				columns: []string{"id", "value"},
				values: [][]driver.Value{
					{int64(1), int64(1)},
					{int64(2), int64(2)},
				},
			}).QueryRowsContext(context.Background(), "q")
		} else {
			binder = NewMapIndexBinder[map[int64]selfRetainingScanner](func(v selfRetainingScanner) int64 {
				return v.Value
			})
			rows, _ = bindTestRows(t, int64(1), int64(2))
		}

		var got map[int64]selfRetainingScanner
		if err := rows.WithBinder(binder).Bind(&got); err != nil {
			t.Fatal(err)
		}
		if got[1].Self == got[2].Self || got[1].Self.Value != 1 || got[2].Self.Value != 2 {
			t.Fatalf("scanner destinations reused: %+v", got)
		}
	}
}

func TestMapScratchResetsNestedValues(t *testing.T) {
	type nested struct {
		Value int64 `sql:"value"`
	}
	type model struct {
		ID    int64   `sql:"id"`
		Child *nested `sql:"child"`
	}

	for _, policy := range []NestedPointerPolicy{AllocateNestedPointers, NilNullNestedPointers} {
		db := bindTestDB(t, &bindFixture{
			columns: []string{"id", "child_value"},
			values: [][]driver.Value{
				{int64(1), int64(7)},
				{int64(2), nil},
				{int64(3), int64(9)},
			},
		}).WithBindConfig(BindConfig{
			Binder: NewMapIndexBinder[map[int64]model](func(v model) int64 { return v.ID }),
			Scan:   ScanOptions{NestedPointers: policy},
		})

		for range 3 {
			var got map[int64]model
			if err := db.QueryRowsContext(context.Background(), "q").Bind(&got); err != nil {
				t.Fatal(err)
			}
			if got[1].Child.Value != 7 || got[3].Child.Value != 9 || got[1].Child == got[3].Child {
				t.Fatal(got)
			}
			if policy == NilNullNestedPointers && got[2].Child != nil {
				t.Fatal("NULL parent retained")
			}
			if policy == AllocateNestedPointers && (got[2].Child == nil || got[2].Child.Value != 0) {
				t.Fatal("previous parent retained")
			}
		}
	}
}

type dynamicStructKey struct{ Value any }

func (v *dynamicStructKey) Scan(src any) error { v.Value = src; return nil }

func TestMapAggregateKeyChecksDynamicComparability(t *testing.T) {
	old := map[dynamicStructKey]struct{}{{Value: "old"}: {}}
	got := old

	r, _ := bindTestRows(t, []byte("not comparable"))
	err := r.WithBinder(NewMapSetBinder[map[dynamicStructKey]struct{}]()).Bind(&got)
	if err == nil {
		t.Fatal("non-comparable aggregate key accepted")
	}
	want := map[dynamicStructKey]struct{}{{Value: "old"}: {}}
	if !reflect.DeepEqual(got, want) || !reflect.DeepEqual(old, want) {
		t.Fatal("destination changed")
	}
}

func TestPreparedScanSurvivesInterleavedBindings(t *testing.T) {
	r, _ := bindTestRows(t, int64(1), int64(2))
	defer r.Close() //nolint:errcheck

	scan, err := PrepareScan(r, reflect.TypeFor[*int64]())
	if err != nil {
		t.Fatal(err)
	}

	for _, want := range []int64{1, 2} {
		if !r.Next() {
			t.Fatal(r.Err())
		}

		// Use and release differently shaped internal scratch while the public
		// prepared function remains live on its original cursor.
		for range 4 {
			other, _ := bindTestRows(t, "hello")
			var words []string
			if err := other.Bind(&words); err != nil {
				t.Fatal(err)
			}
		}

		var got int64
		if err := scan(&got); err != nil || got != want {
			t.Fatalf("%d: %v", got, err)
		}
	}
}

type retainingRowsScanner struct {
	index   int
	targets []*int64
}

func (*retainingRowsScanner) Columns() ([]string, error) { return []string{"value"}, nil }
func (r *retainingRowsScanner) Next() bool               { r.index++; return r.index <= 2 }
func (*retainingRowsScanner) Err() error                 { return nil }
func (r *retainingRowsScanner) Scan(dst ...any) error {
	s := dst[0].(*GeneralScanner)
	r.targets = append(r.targets, s.Value.(*int64))
	return s.Scan(int64(r.index))
}

func TestCustomRowScannerKeepsFreshMapDestinations(t *testing.T) {
	var got map[int64]struct{}
	binding, err := NewMapSetBinder[map[int64]struct{}]().Prepare(&got, BindOptions{})
	if err != nil {
		t.Fatal(err)
	}

	source := &retainingRowsScanner{}
	if err := binding.Scan(source); err != nil {
		t.Fatal(err)
	}

	binding.Commit()
	if len(got) != 2 || source.targets[0] == source.targets[1] ||
		*source.targets[0] != 1 || *source.targets[1] != 2 {
		t.Fatal("custom source observed reused destination storage")
	}
}

func TestScanScratchAcrossWideAndNarrowResults(t *testing.T) {
	for _, width := range []int{9, 65, 1, 9} {
		columns := make([]string, width)
		fields := make([]reflect.StructField, width)
		first, second := make([]driver.Value, width), make([]driver.Value, width)
		for i := range width {
			columns[i] = fmt.Sprintf("f%d", i)
			fields[i] = reflect.StructField{
				Name: fmt.Sprintf("F%d", i),
				Type: reflect.TypeFor[int64](),
				Tag:  reflect.StructTag(fmt.Sprintf("sql:%q", columns[i])),
			}
			first[i], second[i] = int64(i+1), int64(i+101)
		}

		dst := reflect.New(reflect.SliceOf(reflect.StructOf(fields)))
		rows := bindTestDB(t, &bindFixture{
			columns: columns,
			values:  [][]driver.Value{first, second},
		}).QueryRowsContext(context.Background(), "q")
		if err := rows.Bind(dst.Interface()); err != nil {
			t.Fatal(err)
		}
		if dst.Elem().Len() != 2 {
			t.Fatal("wrong result length")
		}

		for i := range width {
			if dst.Elem().Index(0).Field(i).Int() != int64(i+1) ||
				dst.Elem().Index(1).Field(i).Int() != int64(i+101) {
				t.Fatalf("width %d column %d was overwritten", width, i)
			}
		}
	}
}
