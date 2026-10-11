// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"context"
	"fmt"
	"testing"
)

// All paths include the same real [database/sql] fixture, fresh owned results,
// explicit capacity and complete query/scan/close lifecycle. [Rows.Collect] uses the
// registry fallback, without installing the typed binder first.
func BenchmarkCollect(b *testing.B) {
	for _, mode := range []string{"general", "typed", "collect"} {
		b.Run(mode, func(b *testing.B) {
			for _, count := range []int{0, 1, 20, 100, 1000} {
				b.Run(fmt.Sprintf("rows_%d", count), func(b *testing.B) {
					f := &bindFixture{columns: performanceColumns}
					for i := range count {
						f.values = append(f.values, performanceValues(i))
					}

					config := BindConfig{Capacity: count}
					if mode == "general" {
						config.RowsBinder = SliceRowsBinder{}
					}
					if mode == "typed" {
						config.RowsBinder = NewSliceRowsBinder[[]performanceRecord]()
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
