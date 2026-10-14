// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"context"
	"database/sql/driver"
	"fmt"
	"reflect"
	"testing"
	"time"
)

type scanOptionsBenchmarkRecord struct {
	ID    int64     `sql:"id"`
	Stamp time.Time `sql:"stamp"`
}

// Native driver values keep parsing costs out of the measurement. Each loop
// constructs and executes a fresh builder, including result policy capture.
func BenchmarkScanOptionsBinding(b *testing.B) {
	layouts := make([]string, 8)
	for i := range layouts {
		layouts[i] = time.RFC3339Nano
	}
	policies := []struct {
		name    string
		options ScanOptions
	}{
		{"layouts_0", ScanOptions{}},
		{"layouts_2", ScanOptions{TimeLayouts: layouts[:2]}},
		{"layouts_8", ScanOptions{TimeLayouts: layouts}},
		{"location", ScanOptions{Location: time.Local}},
		{"zero_date", ScanOptions{AllowZeroDate: true}},
		{"location_zero_date", ScanOptions{Location: time.Local, AllowZeroDate: true}},
		{"location_zero_date_layouts", ScanOptions{Location: time.Local, AllowZeroDate: true, TimeLayouts: layouts[:2]}},
	}
	typed := NewSliceRowsBinder[[]scanOptionsBenchmarkRecord]()
	inner := NewRowsBinderRegistry()
	inner.RegisterType[*[]scanOptionsBenchmarkRecord](typed)
	middle := NewRowsBinderRegistry()
	middle.RegisterType[*[]scanOptionsBenchmarkRecord](inner)
	outer := NewRowsBinderRegistry()
	outer.RegisterType[*[]scanOptionsBenchmarkRecord](middle)
	composition := ComposeRowsBinders(NewSliceRowsBinder[[]int64](), typed)
	mixed := NewRowsBinderRegistry()
	mixed.RegisterType[*[]scanOptionsBenchmarkRecord](ComposeRowsBinders(
		NewSliceRowsBinder[[]int64](), ComposeRowsBinders(outer),
	))
	for _, tc := range []struct {
		name   string
		binder RowsBinder
		set    bool
		run    func(*Rows) error
	}{
		{
			"typed_slice",
			typed,
			false,
			func(r *Rows) error {
				var v []scanOptionsBenchmarkRecord
				return r.Bind(&v)
			},
		},
		{
			"reflection_slice",
			SliceRowsBinder{},
			false,
			func(r *Rows) error {
				var v []scanOptionsBenchmarkRecord
				return r.Bind(&v)
			},
		},
		{
			"composition",
			composition,
			false,
			func(r *Rows) error {
				var v []scanOptionsBenchmarkRecord
				return r.Bind(&v)
			},
		},
		{
			"nested_registry",
			outer,
			false,
			func(r *Rows) error {
				var v []scanOptionsBenchmarkRecord
				return r.Bind(&v)
			},
		},
		{
			"mixed_routing",
			mixed,
			false,
			func(r *Rows) error {
				var v []scanOptionsBenchmarkRecord
				return r.Bind(&v)
			},
		},
		{
			"composition_custom",
			ComposeRowsBinders(RowsBinderFunc(typed.Prepare)),
			false,
			func(r *Rows) error {
				var v []scanOptionsBenchmarkRecord
				return r.Bind(&v)
			},
		},
		{
			"chunked_slice",
			NewChunkedSliceRowsBinder[[]*scanOptionsBenchmarkRecord](16),
			false,
			func(r *Rows) error {
				var v []*scanOptionsBenchmarkRecord
				return r.Bind(&v)
			},
		},
		{
			"map_pairs",
			NewMapPairsBinder[map[int64]time.Time](),
			false,
			func(r *Rows) error {
				var v map[int64]time.Time
				return r.Bind(&v)
			},
		},
		{
			"map_index",
			NewMapIndexBinder[map[int64]scanOptionsBenchmarkRecord](func(v scanOptionsBenchmarkRecord) int64 { return v.ID }),
			false,
			func(r *Rows) error {
				var v map[int64]scanOptionsBenchmarkRecord
				return r.Bind(&v)
			},
		},
		{
			"map_set",
			NewMapSetBinder[map[time.Time]struct{}](),
			true,
			func(r *Rows) error {
				var v map[time.Time]struct{}
				return r.Bind(&v)
			},
		},
		{
			"custom",
			RowsBinderFunc(typed.Prepare),
			false,
			func(r *Rows) error {
				var v []scanOptionsBenchmarkRecord
				return r.Bind(&v)
			},
		},
		{
			"collect_into",
			nil,
			false,
			func(r *Rows) error {
				_, err := r.CollectInto([]scanOptionsBenchmarkRecord(nil))
				return err
			},
		},
		{
			"visit",
			nil,
			false,
			func(r *Rows) error {
				return r.Visit(func(scanOptionsBenchmarkRecord) (bool, error) {
					return true, nil
				})
			},
		},
		{
			"with_scan",
			nil,
			false,
			benchmarkScanOptionsWithScan},
	} {
		b.Run(tc.name, func(b *testing.B) {
			for _, count := range []int{1, 100} {
				b.Run(fmt.Sprintf("rows_%d", count), func(b *testing.B) {
					for _, policy := range policies {
						b.Run(policy.name, func(b *testing.B) {
							f := &bindFixture{columns: []string{"id", "stamp"}}
							if tc.set {
								f.columns = []string{"stamp"}
							}

							for i := range count {
								stamp := time.Unix(int64(i), 0).UTC()
								values := []driver.Value{int64(i), stamp}
								if tc.set {
									values = []driver.Value{stamp}
								}
								f.values = append(f.values, values)
							}

							db := bindTestDB(b, f).WithBindConfig(BindConfig{
								ScanOptions: policy.options,
								RowsBinder:  tc.binder,
							})

							ctx := context.Background()
							b.ReportAllocs()
							for b.Loop() {
								err := tc.run(db.Select(f.columns...).From("t").QueryRowsContext(ctx))
								if err != nil {
									b.Fatal(err)
								}
							}
						})
					}
				})
			}
		})
	}
}

func benchmarkScanOptionsWithScan(r *Rows) (err error) {
	defer r.closeConsumedRows(&err)
	var v scanOptionsBenchmarkRecord
	args := []any{&v}
	return WithScan(r,
		[]reflect.Type{reflect.TypeFor[*scanOptionsBenchmarkRecord]()},
		func(scan RowScanFunc) error {
			for r.Next() {
				if err := scan(args...); err != nil {
					return err
				}
			}
			return r.Err()
		},
	)
}
