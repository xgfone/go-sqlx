// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"testing"
	"time"
)

type performanceRecord struct {
	ID int64 `sql:"id"`
	A  int64 `sql:"a"`
	B  int64 `sql:"b"`
	C  int64 `sql:"c"`
	D  int64 `sql:"d"`
	E  int64 `sql:"e"`
	F  int64 `sql:"f"`
	G  int64 `sql:"g"`
}

// Each query uses database/sql and fresh output storage. Fixtures are built
// outside the timed loop. The configured binders are shared across queries.
func BenchmarkBindingWorkloads(b *testing.B) {
	type child struct {
		Value int64 `sql:"value"`
	}
	type parent struct {
		ID    int64  `sql:"id"`
		Child *child `sql:"child"`
	}

	cases := []struct {
		name    string
		columns []string
		value   func(int) []driver.Value
		config  BindConfig
		run     func(Rows) error
	}{
		{
			"wide",
			[]string{"id", "a", "b", "c", "d", "e", "f", "g"},
			func(i int) []driver.Value {
				return []driver.Value{int64(i), int64(1), int64(2), int64(3), int64(4), int64(5), int64(6), int64(7)}
			},
			BindConfig{Binder: SliceRowsBinder{}},
			func(r Rows) error { var v []performanceRecord; return r.Bind(&v) },
		},
		{
			"wide_typed",
			[]string{"id", "a", "b", "c", "d", "e", "f", "g"},
			func(i int) []driver.Value {
				return []driver.Value{int64(i), int64(1), int64(2), int64(3), int64(4), int64(5), int64(6), int64(7)}
			},
			BindConfig{Binder: NewSliceRowsBinder[[]performanceRecord]()},
			func(r Rows) error { var v []performanceRecord; return r.Bind(&v) },
		},
		{
			"map_pairs",
			[]string{"id", "value"},
			func(i int) []driver.Value { return []driver.Value{int64(i), int64(i)} },
			BindConfig{Binder: NewMapPairsBinder[map[int64]int64]()},
			func(r Rows) error { var v map[int64]int64; return r.Bind(&v) },
		},
		{
			"map_index",
			[]string{"id", "a"},
			func(i int) []driver.Value { return []driver.Value{int64(i), int64(i)} },
			BindConfig{Binder: NewMapIndexBinder[map[int64]performanceRecord](func(v performanceRecord) int64 { return v.ID })},
			func(r Rows) error { var v map[int64]performanceRecord; return r.Bind(&v) },
		},
		{
			"map_set",
			[]string{"id"},
			func(i int) []driver.Value { return []driver.Value{int64(i)} },
			BindConfig{Binder: NewMapSetBinder[map[int64]struct{}]()},
			func(r Rows) error { var v map[int64]struct{}; return r.Bind(&v) },
		},
		{
			"nullable_parent",
			[]string{"id", "child_value"},
			func(i int) []driver.Value {
				if i%2 == 0 {
					return []driver.Value{int64(i), nil}
				}
				return []driver.Value{int64(i), int64(i)}
			},
			BindConfig{Binder: SliceRowsBinder{}, Scan: ScanOptions{NestedPointers: NilNullNestedPointers}},
			func(r Rows) error { var v []parent; return r.Bind(&v) },
		},
		{
			"pointer_duration",
			[]string{"value"},
			func(i int) []driver.Value { return []driver.Value{int64(i)} },
			BindConfig{Binder: SliceRowsBinder{}},
			func(r Rows) error { var v []*time.Duration; return r.Bind(&v) }},
		{
			"bytes",
			[]string{"value"},
			func(int) []driver.Value { return []driver.Value{[]byte("a driver-owned byte buffer")} },
			BindConfig{},
			func(r Rows) error { var v [][]byte; return r.Bind(&v) },
		},
		{
			"custom_scanner_map",
			[]string{"id", "value"},
			func(i int) []driver.Value { return []driver.Value{int64(i), int64(i)} },
			BindConfig{Binder: NewMapPairsBinder[map[int64]sql.NullInt64]()},
			func(r Rows) error { var v map[int64]sql.NullInt64; return r.Bind(&v) },
		},
	}

	for _, c := range cases {
		b.Run(c.name, func(b *testing.B) {
			for _, n := range []int{0, 1, 1000} {
				b.Run(fmt.Sprintf("rows_%d", n), func(b *testing.B) {
					f := &bindFixture{columns: c.columns}
					for i := range n {
						f.values = append(f.values, c.value(i))
					}

					db := bindTestDB(b, f).WithBindConfig(c.config)
					b.ReportAllocs()
					for b.Loop() {
						if err := c.run(db.QueryRowsContext(context.Background(), "q")); err != nil {
							b.Fatal(err)
						}
					}
				})
			}
		})
	}
}

// Capacity is an explicit caller hint; query limits and table cardinality are
// not reliable reasons for the library to reserve arbitrarily large buffers.
func BenchmarkBindingCapacity(b *testing.B) {
	f := &bindFixture{columns: []string{"id", "a"}}
	for i := range 1000 {
		f.values = append(f.values, []driver.Value{int64(i), int64(i)})
	}

	for _, capacity := range []int{0, 1000} {
		b.Run(fmt.Sprintf("capacity_%d", capacity), func(b *testing.B) {
			db := bindTestDB(b, f).WithBindConfig(BindConfig{Capacity: capacity,
				Binder: NewMapIndexBinder[map[int64]performanceRecord](func(v performanceRecord) int64 {
					return v.ID
				}),
			})

			b.ReportAllocs()
			for b.Loop() {
				var got map[int64]performanceRecord
				if err := db.QueryRowsContext(context.Background(), "q").Bind(&got); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

var performanceBuilder *SelectBuilder

func BenchmarkBindingSetup(b *testing.B) {
	b.Run("oper_select_layouts", func(b *testing.B) {
		o := NewOper[performanceRecord]("t").WithBindConfig(BindConfig{
			Scan: ScanOptions{TimeLayouts: []string{time.RFC3339Nano, time.DateOnly}},
		})
		b.ReportAllocs()
		for b.Loop() {
			performanceBuilder = o.Select("id")
		}
	})

	b.Run("manual", func(b *testing.B) {
		f := &bindFixture{columns: []string{"id", "a"}}
		for i := range 1000 {
			f.values = append(f.values, []driver.Value{int64(i), int64(i)})
		}

		db := bindTestDB(b, f)
		b.ReportAllocs()
		for b.Loop() {
			var v performanceRecord
			r := db.QueryRowsContext(context.Background(), "q")
			for r.Next() {
				if err := r.Scan(&v); err != nil {
					b.Fatal(err)
				}
			}
			if err := r.Err(); err != nil {
				b.Fatal(err)
			}
			if err := r.Close(); err != nil {
				b.Fatal(err)
			}
		}
	})
}
