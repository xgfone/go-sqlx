// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package rowbind

import (
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"
)

func TestPrepareSharesImmutableOptionsAndSnapshotsShape(t *testing.T) {
	type record struct {
		Stamp time.Time `sql:"stamp"`
	}

	options := ScanOptions{TimeLayouts: []string{"02/01/2006"}}
	for _, typ := range []reflect.Type{
		reflect.TypeFor[*time.Time](),
		reflect.TypeFor[*record](),
	} {
		columns, types := []string{"stamp"}, []reflect.Type{typ}
		mapping, err := Prepare(columns, types, options)
		if err != nil {
			t.Fatal(err)
		}
		if mapping.options == nil || &mapping.options.TimeLayouts[0] != &options.TimeLayouts[0] {
			t.Fatal("preparation copied the immutable option slice")
		}

		columns[0], types[0] = "changed", reflect.TypeFor[*int]()
		cursor := &mappingCursor{scanCacheSource("11/09/2026")}
		dst := reflect.New(typ.Elem()).Interface()
		err = mapping.WithScan(cursor, func(scan RowScanFunc, _ Reuse) error {
			return scan(dst)
		})

		stamp, scalar := dst.(*time.Time)
		if !scalar {
			stamp = &dst.(*record).Stamp
		}

		if err != nil || stamp.Day() != 11 || options.TimeLayouts[0] != "02/01/2006" {
			t.Fatal("preparation changed its inputs or retained mutable shape", dst, err)
		}
	}
}

func TestPrepareValidatesOptions(t *testing.T) {
	for _, options := range []ScanOptions{
		{Nulls: 99},
		{NestedPointers: 99},
		{DurationUnit: -1},
	} {
		_, err := Prepare([]string{"value"}, []reflect.Type{reflect.TypeFor[*int]()}, options)
		if err == nil {
			t.Fatal("invalid options accepted", options)
		}
	}
}

func TestPrepareFailureReturnsZeroMapping(t *testing.T) {
	type record struct {
		Value int64 `sql:"value"`
	}
	for _, shape := range []struct {
		name    string
		columns []string
		types   []reflect.Type
	}{
		{"column_count", []string{"first", "second"}, []reflect.Type{reflect.TypeFor[*int64]()}},
		{"destination", []string{"value"}, []reflect.Type{reflect.TypeFor[int64]()}},
		{"scalar", []string{"value"}, []reflect.Type{reflect.TypeFor[*complex128]()}},
		{"layout", []string{"unknown"}, []reflect.Type{reflect.TypeFor[*record]()}},
	} {
		t.Run(shape.name, func(t *testing.T) {
			for _, options := range []ScanOptions{
				{},
				{Location: time.Local, AllowZeroDate: true, TimeLayouts: []string{time.DateTime}},
				{Location: time.Local, AllowZeroDate: true, DurationUnit: time.Second},
				{Location: time.Local, AllowZeroDate: true, Nulls: NullError},
			} {
				mapping, err := Prepare(shape.columns, shape.types, options)
				if err == nil || mapping.flags != 0 || mapping.options != nil {
					t.Fatal("invalid preparation returned usable state", mapping, err)
				}
			}
		})
	}
}

func TestPrepareFailureDoesNotAllocateOptionsSnapshot(t *testing.T) {
	type record struct{ Value int64 }
	columns, types := []string{"value"}, []reflect.Type{reflect.TypeFor[record]()}
	for _, options := range []ScanOptions{
		{},
		{Location: time.Local, AllowZeroDate: true, TimeLayouts: []string{time.DateTime}},
		{Location: time.Local, AllowZeroDate: true, DurationUnit: time.Second},
		{Location: time.Local, AllowZeroDate: true, Nulls: NullError},
	} {
		// A non-pointer struct returns a fixed error without fmt's scratch pool,
		// whose allocations are intentionally unpredictable with race enabled.
		allocs := testing.AllocsPerRun(100, func() {
			if _, err := Prepare(columns, types, options); err == nil {
				t.Fatal("invalid preparation accepted")
			}
		})
		if allocs != 1 {
			t.Fatalf("failed preparation allocated %g times, want only its error", allocs)
		}
	}
}

func TestPrepareSnapshotsTimePolicies(t *testing.T) {
	type record struct {
		Stamp time.Time     `sql:"stamp"`
		Span  time.Duration `sql:"span"`
		Other time.Time     `sql:"other"`
	}

	zone := time.FixedZone("test", 8*60*60)
	native := time.Date(2026, 10, 11, 8, 30, 0, 0, time.UTC)
	for _, tc := range []struct {
		name    string
		options ScanOptions
	}{
		{"location", ScanOptions{Location: zone}},
		{"zero_date", ScanOptions{AllowZeroDate: true}},
		{"location_zero_date", ScanOptions{Location: zone, AllowZeroDate: true}},
		{"layouts", ScanOptions{Location: zone, AllowZeroDate: true, TimeLayouts: []string{"02/01/2006 15:04:05"}}},
		{"duration", ScanOptions{Location: zone, AllowZeroDate: true, DurationUnit: time.Second}},
		{"nulls", ScanOptions{Location: zone, AllowZeroDate: true, Nulls: NullError}},
		{"default", ScanOptions{}},
	} {
		for _, shape := range []string{"scalar_pair", "scalar_wide", "struct"} {
			for _, method := range []string{"with_scan", "scanner"} {
				t.Run(tc.name+"/"+shape+"/"+method, func(t *testing.T) {
					options := tc.options
					columns := []string{"stamp", "span", "other"}
					types := []reflect.Type{
						reflect.TypeFor[*time.Time](),
						reflect.TypeFor[*time.Duration](),
						reflect.TypeFor[*time.Time](),
					}

					switch shape {
					case "scalar_pair":
						columns, types = columns[:2], types[:2]
					case "struct":
						types = []reflect.Type{reflect.TypeFor[*record]()}
					}

					mapping, err := Prepare(columns, types, options)
					if err != nil {
						t.Fatal(err)
					}

					// Reassigning caller-owned fields must not change prepared policies.
					options.Location = time.FixedZone("changed", -5*60*60)
					options.AllowZeroDate = !options.AllowZeroDate
					options.TimeLayouts = []string{"invalid"}
					options.DurationUnit = time.Hour
					options.Nulls = NullError - options.Nulls

					loc := tc.options.Location
					if loc == nil {
						loc = time.UTC
					}

					layout := time.DateTime
					if len(tc.options.TimeLayouts) != 0 {
						layout = tc.options.TimeLayouts[0]
					}

					wantNative := native
					if tc.options.Location != nil {
						wantNative = native.In(loc)
					}

					for _, source := range []struct {
						name  string
						value any
						want  time.Time
						fails bool
					}{
						{"native", native, wantNative, false},
						{"text", native.Format(layout), time.Date(2026, 10, 11, 8, 30, 0, 0, loc), false},
						{"zero_date", "0000-00-00 00:00:00", time.Time{}, !tc.options.AllowZeroDate},
						{"null", nil, time.Time{}, tc.options.Nulls == NullError},
					} {
						t.Run(source.name, func(t *testing.T) {
							values := []any{source.value, int64(2), native}
							var got record
							args := []any{&got.Stamp, &got.Span, &got.Other}

							switch shape {
							case "scalar_pair":
								values, args = values[:2], args[:2]
							case "struct":
								args = []any{&got}
							}

							cursor := &mappingCursor{scanCacheSource(values...)}
							copy := mapping
							if method == "with_scan" {
								err = copy.WithScan(cursor, func(scan RowScanFunc, _ Reuse) error { return scan(args...) })
							} else {
								scanner, e := copy.Scanner(cursor.Scan)
								if e != nil {
									t.Fatal(e)
								}

								err = scanner.Scan(args...)
								if e = scanner.Close(); e != nil {
									t.Fatal(e)
								}
							}
							if (err != nil) != source.fails {
								t.Fatalf("unexpected scan error: %v", err)
							}
							if source.fails {
								return
							}

							unit := tc.options.DurationUnit
							if unit == 0 {
								unit = time.Millisecond
							}
							if got.Stamp != source.want || got.Span != 2*unit ||
								(shape != "scalar_pair" && got.Other != wantNative) {
								t.Fatal("prepared policies changed", got)
							}
						})
					}
				})
			}
		}
	}
}

func TestMappingSharesCompactPoliciesConcurrently(t *testing.T) {
	zone := time.FixedZone("test", 8*60*60)
	columns, types := []string{"stamp"}, []reflect.Type{reflect.TypeFor[*time.Time]()}
	compact, err := Prepare(columns, types, ScanOptions{Location: zone, AllowZeroDate: true})
	if err != nil {
		t.Fatal(err)
	}

	defaults, err := Prepare(columns, types, ScanOptions{})
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 8 {
				for _, tc := range []struct {
					mapping Mapping
					loc     *time.Location
					allow   bool
				}{
					{compact, zone, true}, {defaults, time.UTC, false},
				} {
					for _, source := range []string{"0000-00-00 00:00:00", "2026-10-11 08:30:00"} {
						cursor := &mappingCursor{scanCacheSource(source)}
						var got time.Time
						err := tc.mapping.WithScan(cursor, func(scan RowScanFunc, _ Reuse) error { return scan(&got) })
						if source == "0000-00-00 00:00:00" {
							if (err == nil) != tc.allow || !got.IsZero() {
								t.Error("execution shared or leaked zero-date policy", got, err)
							}
						} else if err != nil || got.Location() != tc.loc {
							t.Error("execution shared or leaked location", got, err)
						}
					}
				}
			}
		})
	}
	wg.Wait()
}

var mappingOptionsAllocationSink Mapping

func TestPrepareTimePolicyAllocations(t *testing.T) {
	type record struct {
		Stamp time.Time `sql:"stamp"`
	}

	columns, types := []string{"stamp"}, []reflect.Type{reflect.TypeFor[*record]()}
	for _, tc := range []struct {
		name    string
		options ScanOptions
		want    float64
	}{
		{"default", ScanOptions{}, 0},
		{"location", ScanOptions{Location: time.Local}, 0},
		{"zero_date", ScanOptions{AllowZeroDate: true}, 0},
		{"location_zero_date", ScanOptions{Location: time.Local, AllowZeroDate: true}, 0},
		{"mapping_policies", ScanOptions{IgnoreUnknownColumns: true, NestedPointers: NilNullNestedPointers}, 0},
		{"layouts", ScanOptions{Location: time.Local, AllowZeroDate: true, TimeLayouts: []string{time.DateTime}}, 1},
		{"duration", ScanOptions{Location: time.Local, AllowZeroDate: true, DurationUnit: time.Second}, 1},
		{"nulls", ScanOptions{Location: time.Local, AllowZeroDate: true, Nulls: NullError}, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			allocs := testing.AllocsPerRun(100, func() {
				mapping, err := Prepare(columns, types, tc.options)
				if err != nil {
					panic(fmt.Sprintf("unexpected preparation failure: %v", err))
				}
				mappingOptionsAllocationSink = mapping
			})
			if allocs != tc.want {
				t.Fatalf("prepared mapping allocated %g times, want %g", allocs, tc.want)
			}
		})
	}
}
