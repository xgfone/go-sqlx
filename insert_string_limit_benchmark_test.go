// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"fmt"
	"strings"
	"testing"
)

func BenchmarkInsertStringLimitCosts(b *testing.B) {
	type limited struct {
		ID     int64
		Name   string `sql:"Name,maxlen=64,overflow=truncate"`
		Active bool
	}
	for _, count := range []int{1, 100} {
		b.Run(fmt.Sprintf("rows%d", count), func(b *testing.B) {
			plain := make([]insertBenchmarkRecord, count)
			valid, long := make([]limited, count), make([]limited, count)
			for i := range plain {
				plain[i] = insertBenchmarkRecord{int64(i + 1000), "alice", true}
				valid[i] = limited{int64(i + 1000), "alice", true}
				long[i] = limited{int64(i + 1000), strings.Repeat("a", 128), true}
			}

			b.Run("plain", func(b *testing.B) { benchmarkStringLimitRows(b, plain) })
			b.Run("valid", func(b *testing.B) { benchmarkStringLimitRows(b, valid) })
			b.Run("truncate", func(b *testing.B) { benchmarkStringLimitRows(b, long) })
		})
	}
}

func benchmarkStringLimitRows[T any](b *testing.B, rows []T) {
	b.Helper()
	plan, err := CompileInsert[T]()
	if err != nil {
		b.Fatal(err)
	}

	for _, mode := range []string{"struct", "structs", "plan"} {
		b.Run(mode, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				q := Insert().Into("records")
				switch mode {
				case "struct":
					for _, row := range rows {
						q.Struct(row)
					}

				case "structs":
					q.Structs(rows)

				case "plan":
					if err := plan.AppendTo(q, rows); err != nil {
						b.Fatal(err)
					}
				}

				if q.err != nil {
					b.Fatal(q.err)
				}
				insertBatchResult = q
			}
		})
	}
}
