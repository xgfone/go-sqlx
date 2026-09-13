// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"fmt"
	"testing"
)

// Prebuilt cell values keep boxing and model metadata outside the measurement.
// Bulk reserves a complete batch; prefix appends after a known 1000-row batch.
func BenchmarkInsertGrowth(b *testing.B) {
	for _, width := range []int{3, 12, 64, 256} {
		for _, count := range []int{0, 1, 20, 100, 1000} {
			for _, mode := range []string{"incremental", "bulk", "prefix"} {
				b.Run(fmt.Sprintf("cols%d/rows%d/%s", width, count, mode), func(b *testing.B) {
					values := make([]any, width)
					for i := range values {
						values[i] = int64(i + 1000)
					}

					makeBuilder := func() *InsertBuilder {
						q := Insert().Into("records")
						if mode == "bulk" || mode == "prefix" {
							rows := count
							if mode == "prefix" {
								rows = 1000
							}
							if rows > 0 {
								cells := q.values.nextRows(rows, width)
								for i := 0; i < len(cells); i += width {
									copy(cells[i:], values)
								}
								q.values.commitRows(rows, width)
							}
						}

						if mode != "bulk" {
							for range count {
								q.Values(values...)
							}
						}
						return q
					}

					builder := makeBuilder()
					for _, phase := range []string{"construct", "build", "construct_build", "clone"} {
						if count == 0 && mode != "prefix" && phase != "construct" && phase != "clone" {
							continue
						}

						b.Run(phase, func(b *testing.B) {
							b.ReportAllocs()
							for b.Loop() {
								q := builder
								if phase == "construct" || phase == "construct_build" {
									q = makeBuilder()
								}
								if q.err != nil {
									b.Fatal(q.err)
								}

								switch phase {
								case "construct":
									insertBatchResult = q

								case "clone":
									insertBatchResult = q.Clone()

								default:
									var err error
									insertSQLResult, insertArgsResult, err = q.Build()
									if err != nil {
										b.Fatal(err)
									}
								}
							}
						})
					}
				})
			}
		}
	}
}
