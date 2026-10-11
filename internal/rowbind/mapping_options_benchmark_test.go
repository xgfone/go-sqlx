// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package rowbind

import (
	"fmt"
	"reflect"
	"testing"
	"time"
)

var mappingOptionsBenchmarkSink Mapping

func BenchmarkMappingOptionsPreparationFailure(b *testing.B) {
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
		{"layout", []string{"unknown"}, []reflect.Type{reflect.TypeFor[*record]()}},
	} {
		b.Run(shape.name, func(b *testing.B) {
			for _, policy := range []struct {
				name    string
				options ScanOptions
			}{
				{"default", ScanOptions{}},
				{"location_zero_date", ScanOptions{Location: time.Local, AllowZeroDate: true}},
				{"layouts", ScanOptions{TimeLayouts: []string{time.DateTime}}},
				{"duration", ScanOptions{DurationUnit: time.Second}},
				{"nulls", ScanOptions{Nulls: NullError}},
			} {
				b.Run(policy.name, func(b *testing.B) {
					b.ReportAllocs()
					for b.Loop() {
						_, err := Prepare(shape.columns, shape.types, policy.options)
						if err == nil {
							b.Fatal("invalid preparation accepted")
						}
					}
				})
			}
		})
	}
}

func BenchmarkMappingOptions(b *testing.B) {
	columns := []string{"value"}
	types := []reflect.Type{reflect.TypeFor[*time.Time]()}
	for _, n := range []int{0, 2, 8} {
		b.Run(fmt.Sprintf("layouts_%d", n), func(b *testing.B) {
			options := ScanOptions{TimeLayouts: make([]string, n)}
			b.ReportAllocs()
			for b.Loop() {
				mapping, err := Prepare(columns, types, options)
				if err != nil {
					b.Fatal(err)
				}
				mappingOptionsBenchmarkSink = mapping
			}
		})
	}
	for _, tc := range []struct {
		name    string
		options ScanOptions
	}{
		{"location", ScanOptions{Location: time.Local}},
		{"zero_date", ScanOptions{AllowZeroDate: true}},
		{"location_zero_date", ScanOptions{Location: time.Local, AllowZeroDate: true}},
		{"location_zero_date_layouts", ScanOptions{Location: time.Local, AllowZeroDate: true, TimeLayouts: []string{time.DateTime}}},
	} {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				mapping, err := Prepare(columns, types, tc.options)
				if err != nil {
					b.Fatal(err)
				}
				mappingOptionsBenchmarkSink = mapping
			}
		})
	}
}
