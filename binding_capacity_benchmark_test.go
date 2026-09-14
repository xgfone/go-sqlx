// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"context"
	"database/sql/driver"
	"math"
	"testing"
)

// Compare bounded LIMIT hints with the default and an explicit large capacity.
// The fixture respects each query's limit but does not execute the SQL itself.
func BenchmarkBindingLimitCapacity(b *testing.B) {
	for _, kind := range []string{"typed", "general", "map"} {
		b.Run(kind, func(b *testing.B) {
			for _, tc := range []struct {
				name     string
				rows     int
				limit    int64 // -1 leaves LIMIT unset.
				capacity int
			}{
				{"default_rows100", 100, -1, 0},
				{"limit100_rows0", 0, 100, 0},
				{"limit100_rows1", 1, 100, 0},
				{"limit100_rows20", 20, 100, 0},
				{"limit100_rows100", 100, 100, 0},
				{"limit1000_rows1", 1, 1000, 0},
				{"limit1000_rows1000", 1000, 1000, 0},
				{"limit_max_rows1", 1, math.MaxInt64, 0},
				{"explicit1000_rows1000", 1000, 1000, 1000},
			} {
				b.Run(tc.name, func(b *testing.B) {
					f := &bindFixture{columns: []string{"id", "a", "b", "c", "d", "e", "f", "g"}}
					for i := range tc.rows {
						f.values = append(f.values, []driver.Value{
							int64(i), int64(1), int64(2), int64(3),
							int64(4), int64(5), int64(6), int64(7),
						})
					}

					binder := NewSliceRowsBinder[[]performanceRecord]()
					switch kind {
					case "general":
						binder = SliceRowsBinder{}

					case "map":
						binder = NewMapIndexBinder[map[int64]performanceRecord](func(v performanceRecord) int64 {
							return v.ID
						})
					}

					db := bindTestDB(b, f).WithBindConfig(BindConfig{
						RowsBinder: binder,
						Capacity:   tc.capacity,
					})
					q := db.Select("*").From("records")
					if tc.limit >= 0 {
						q.Limit(tc.limit)
					}

					ctx := context.Background()
					b.ReportAllocs()
					for b.Loop() {
						r := q.QueryRowsContext(ctx)
						if kind == "map" {
							var got map[int64]performanceRecord
							if err := r.Bind(&got); err != nil || len(got) != tc.rows {
								b.Fatal("invalid result", err, len(got))
							}
						} else {
							var got []performanceRecord
							if err := r.Bind(&got); err != nil || len(got) != tc.rows {
								b.Fatal("invalid result", err, len(got))
							}
						}
					}
				})
			}
		})
	}
}
