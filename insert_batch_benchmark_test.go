// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"testing"
)

type insertBenchmarkRecord struct {
	ID     int64
	Name   string
	Active bool
}

type insertBenchmarkWide struct {
	insertBenchmarkRecord
	A, B, C, D, E, F, G, H, I int64
}

var insertSQLResult string
var insertArgsResult []any

// Fixtures are prepared outside timing. [InsertBuilder.Build] and execution reuse a builder;
// construct and construct_build include each batch's snapshot allocations.
func BenchmarkInsertBatchWorkloads(b *testing.B) {
	for _, width := range []int{3, 12} {
		for _, n := range []int{1, 20, 100, 1000} {
			b.Run(fmt.Sprintf("cols%d/rows%d", width, n), func(b *testing.B) {
				var input any
				if width == 3 {
					rows := make([]insertBenchmarkRecord, n)
					for i := range rows {
						rows[i] = insertBenchmarkRecord{int64(i + 1000), "alice", true}
					}
					input = rows
				} else {
					rows := make([]insertBenchmarkWide, n)
					for i := range rows {
						rows[i] = insertBenchmarkWide{
							insertBenchmarkRecord{int64(i + 1000), "alice", true},
							1000, 1001, 1002, 1003, 1004, 1005, 1006, 1007, 1008,
						}
					}
					input = rows
				}
				runInsertBatchWorkloads(b, n*width, func() *InsertBuilder {
					return Insert().Into("records").Structs(input)
				})
			})
		}
	}
}

func runInsertBatchWorkloads(b *testing.B, argCount int, makeBuilder func() *InsertBuilder, modes ...string) {
	b.Helper()
	builder := makeBuilder()
	if _, _, err := builder.Build(); err != nil {
		b.Fatal(err)
	}

	builder.SetExecutor(templateTestExecutor{
		exec: func(_ context.Context, _ string, args ...any) (sql.Result, error) {
			if len(args) != argCount {
				b.Fatalf("got %d arguments, want %d", len(args), argCount)
			}
			return driver.RowsAffected(1), nil
		},
	})

	ctx := context.Background()
	if len(modes) == 0 {
		modes = []string{"construct", "build", "construct_build", "exec"}
	}
	for _, mode := range modes {
		b.Run(mode, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				q := builder
				if mode == "construct" || mode == "construct_build" {
					q = makeBuilder()
				}

				var err error
				switch mode {
				case "construct":
					insertBatchResult, err = q, q.err

				case "build", "construct_build":
					insertSQLResult, insertArgsResult, err = q.Build()

				case "exec":
					_, err = q.ExecContext(ctx)
				}

				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
