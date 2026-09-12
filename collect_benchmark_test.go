// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"context"
	"database/sql/driver"
	"fmt"
	"testing"
)

// All paths include the same real database/sql fixture, fresh owned results,
// explicit capacity and complete query/scan/close lifecycle. Collect uses the
// registry fallback, without installing the typed binder first.
func BenchmarkCollect(b *testing.B) {
	for _, mode := range []string{"general", "typed", "collect"} {
		b.Run(mode, func(b *testing.B) {
			for _, count := range []int{0, 1, 20, 100, 1000} {
				b.Run(fmt.Sprintf("rows_%d", count), func(b *testing.B) {
					f := &bindFixture{columns: []string{"id", "a", "b", "c", "d", "e", "f", "g"}}
					for i := range count {
						f.values = append(f.values, []driver.Value{
							int64(i), int64(1), int64(2), int64(3),
							int64(4), int64(5), int64(6), int64(7),
						})
					}

					config := BindConfig{Capacity: count}
					if mode == "general" {
						config.Binder = SliceRowsBinder{}
					}
					if mode == "typed" {
						config.Binder = NewSliceRowsBinder[[]performanceRecord]()
					}

					db := bindTestDB(b, f).WithBindConfig(config)
					run := (*Rows).Collect[performanceRecord]
					if mode != "collect" {
						run = func(rows *Rows) ([]performanceRecord, error) {
							var values []performanceRecord
							err := rows.Bind(&values)
							return values, err
						}
					}

					b.ReportAllocs()
					for b.Loop() {
						rows := db.QueryRowsContext(context.Background(), "q")
						got, err := run(rows)
						if err != nil || len(got) != count {
							b.Fatal(got, err)
						}
					}
				})
			}
		})
	}
}
