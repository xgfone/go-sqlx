// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"testing"

	"github.com/xgfone/go-sqlx/dialect"
)

func BenchmarkInsertTemplate(b *testing.B) {
	for _, count := range []int{1, 100, 1000} {
		b.Run(fmt.Sprintf("rows%d", count), func(b *testing.B) {
			params := []any{int64(1), int64(2), int64(3)}
			slots := []any{Param(0), Param(1), Param(2)}
			db := &DB{
				Dialect: dialect.Postgres,
				Executor: templateTestExecutor{
					exec: func(_ context.Context, _ string, args ...any) (sql.Result, error) {
						if len(args) != 3*count || args[0] != params[0] || args[len(args)-1] != params[2] {
							b.Fatal("invalid template arguments")
						}
						return driver.RowsAffected(1), nil
					},
				},
			}

			builder := db.Insert().Into("records")
			for range count {
				builder.Values(slots...)
			}
			compiled := mustCompileTemplate(b, builder)
			ctx := context.Background()

			for _, mode := range []string{"compile", "bind", "exec"} {
				b.Run(mode, func(b *testing.B) {
					b.ReportAllocs()
					for b.Loop() {
						var err error
						switch mode {
						case "compile":
							templateSink, err = builder.Compile()

						case "bind":
							templateSQLSink, templateArgsSink, err = compiled.Bind(params...)

						default:
							_, err = compiled.ExecContext(ctx, db, params...)
						}

						if err != nil {
							b.Fatal(err)
						}
					}
				})
			}
		})
	}
}

// Incremental VALUES complements the flat [InsertBuilder.Structs]/[InsertPlan] workloads.
// Inputs
// are prebuilt; construction still owns a snapshot of every appended row.
func BenchmarkInsertValueWorkloads(b *testing.B) {
	for _, kind := range []string{"plain", "expressions", "mixed"} {
		for _, width := range []int{3, 12, 64} {
			for _, count := range []int{1, 100, 1000} {
				b.Run(fmt.Sprintf("%s/cols%d/rows%d", kind, width, count), func(b *testing.B) {
					values := make([]any, width)
					args := 0
					for i := range values {
						switch {
						case kind == "expressions":
							values[i] = Value(int64(i + 1000))
							args++

						case kind == "mixed" && i%3 == 1:
							values[i] = Expr("? + ?", int64(i), int64(i+1))
							args += 2

						case kind == "mixed" && i%3 == 2:
							values[i] = Default()

						default:
							values[i] = int64(i + 1000)
							args++
						}
					}

					runInsertBatchWorkloads(b, args*count, func() *InsertBuilder {
						q := Insert().SetDialect(dialect.Postgres).Into("records")
						for range count {
							q.Values(values...)
						}
						return q
					})
				})
			}
		}
	}
}
