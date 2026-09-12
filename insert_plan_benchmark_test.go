// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"fmt"
	"testing"
)

func BenchmarkInsertPlanWorkloads(b *testing.B) {
	for _, width := range []int{3, 12} {
		for _, n := range []int{1, 20, 100, 1000} {
			b.Run(fmt.Sprintf("cols%d/rows%d", width, n), func(b *testing.B) {
				if width == 3 {
					rows := make([]insertBenchmarkRecord, n)
					for i := range rows {
						rows[i] = insertBenchmarkRecord{int64(i + 1000), "alice", true}
					}
					runInsertPlanWorkloads(b, rows, width)
				} else {
					rows := make([]insertBenchmarkWide, n)
					for i := range rows {
						rows[i] = insertBenchmarkWide{
							insertBenchmarkRecord{int64(i + 1000), "alice", true},
							1000, 1001, 1002, 1003, 1004, 1005, 1006, 1007, 1008,
						}
					}
					runInsertPlanWorkloads(b, rows, width)
				}
			})
		}
	}
}

func runInsertPlanWorkloads[T any](b *testing.B, rows []T, width int) {
	b.Helper()
	p, err := CompileInsert[T]()
	if err != nil {
		b.Fatal(err)
	}

	runInsertBatchWorkloads(b, len(rows)*width, func() *InsertBuilder {
		q := Insert().Into("records")
		if err := p.AppendTo(q, rows); err != nil {
			b.Fatal(err)
		}
		return q
	})
}

var insertPlanResult any

func BenchmarkCompileInsert(b *testing.B) {
	for _, width := range []int{3, 12} {
		b.Run(fmt.Sprintf("cols%d", width), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				var err error
				if width == 3 {
					insertPlanResult, err = CompileInsert[insertBenchmarkRecord]()
				} else {
					insertPlanResult, err = CompileInsert[insertBenchmarkWide]()
				}
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkInsertPlanInputs(b *testing.B) {
	rows := make([]insertBenchmarkRecord, 100)
	ptrs := make([]*insertBenchmarkRecord, len(rows))
	values := make([]struct {
		V  pointerValue
		ID int64
	}, len(rows))
	for i := range rows {
		rows[i] = insertBenchmarkRecord{int64(i + 1000), "alice", true}
		ptrs[i] = &rows[i]
		values[i].V, values[i].ID = pointerValue{i + 1000}, int64(i+1000)
	}

	b.Run("values", func(b *testing.B) { benchmarkInsertPlanInput(b, rows) })
	b.Run("pointers", func(b *testing.B) { benchmarkInsertPlanInput(b, ptrs) })
	b.Run("pointer_valuer", func(b *testing.B) { benchmarkInsertPlanInput(b, values) })
}

func benchmarkInsertPlanInput[T any](b *testing.B, rows []T) {
	b.Helper()
	p, err := CompileInsert[T]()
	if err != nil {
		b.Fatal(err)
	}

	for _, mode := range []string{"structs", "plan"} {
		b.Run(mode, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				q := Insert().Into("records")
				if mode == "structs" {
					q.Structs(rows)
				} else if err := p.AppendTo(q, rows); err != nil {
					b.Fatal(err)
				}
				if q.err != nil {
					b.Fatal(q.err)
				}
				insertBatchResult = q
			}
		})
	}
}
