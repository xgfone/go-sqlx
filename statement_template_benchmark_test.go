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

var templateSQLSink string
var templateArgsSink []any
var templateSink *StatementTemplate

func BenchmarkStatementTemplate(b *testing.B) {
	for _, n := range []int{1, 24, 128} {
		b.Run(fmt.Sprintf("params%d", n), func(b *testing.B) {
			params, slots := make([]any, n), make([]any, n)
			for i := range params {
				params[i], slots[i] = int64(i+1000), Param(i)
			}

			makeQuery := func(values []any) *SelectBuilder {
				return Select("id", "name").From("users").Where(In("id", values...)).
					SetDialect(dialect.Postgres)
			}

			builder, declaration := makeQuery(params), makeQuery(slots)
			compiled := mustCompileTemplate(b, declaration)
			b.Run("build", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					var err error
					templateSQLSink, templateArgsSink, err = builder.Build()
					if err != nil {
						b.Fatal(err)
					}
				}
			})

			b.Run("construct_build", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					var err error
					templateSQLSink, templateArgsSink, err = makeQuery(params).Build()
					if err != nil {
						b.Fatal(err)
					}
				}
			})

			b.Run("compile", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					var err error
					templateSink, err = declaration.Compile()
					if err != nil {
						b.Fatal(err)
					}
				}
			})

			b.Run("construct_compile", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					values := make([]any, n)
					for i := range values {
						values[i] = Param(i)
					}

					var err error
					templateSink, err = makeQuery(values).Compile()
					if err != nil {
						b.Fatal(err)
					}
				}
			})

			b.Run("bind", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					var err error
					templateSQLSink, templateArgsSink, err = compiled.Bind(params...)
					if err != nil {
						b.Fatal(err)
					}
				}
			})
		})
	}
}

// Isolate preparation and dispatch, without network/server/driver work. The
// normal builder already borrows its args, as does the template execution path.
func BenchmarkStatementTemplateExec(b *testing.B) {
	for _, n := range []int{1, 24, 128} {
		b.Run(fmt.Sprintf("params%d", n), func(b *testing.B) {
			params, slots := make([]any, n), make([]any, n)
			for i := range params {
				params[i], slots[i] = int64(i+1000), Param(i)
			}

			db := &DB{Dialect: dialect.Postgres, Executor: templateTestExecutor{
				exec: func(_ context.Context, _ string, args ...any) (sql.Result, error) {
					if len(args) != n || args[0] != params[0] || args[n-1] != params[n-1] {
						b.Fatal("invalid execution arguments")
					}
					return driver.RowsAffected(1), nil
				},
			}}

			builder := db.Delete().From("users").Where(In("id", params...))
			compiled := mustCompileTemplate(b, db.Delete().From("users").Where(In("id", slots...)))
			ctx := context.Background()
			for _, mode := range []string{"builder", "template"} {
				b.Run(mode, func(b *testing.B) {
					b.ReportAllocs()
					for b.Loop() {
						var err error
						if mode == "builder" {
							_, err = builder.ExecContext(ctx)
						} else {
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

// Exercise compiled queries through database/sql and result binding. Fixtures
// and declarations are outside timing; each iteration creates a fresh result.
func BenchmarkStatementTemplateRows(b *testing.B) {
	for _, n := range []int{0, 1, 20, 100, 1000} {
		b.Run(fmt.Sprintf("rows%d", n), func(b *testing.B) {
			f := &bindFixture{columns: []string{"id", "a", "b", "c", "d", "e", "f", "g"}}
			for i := range n {
				f.values = append(f.values, []driver.Value{
					int64(i), int64(1), int64(2), int64(3), int64(4), int64(5), int64(6), int64(7),
				})
			}
			db := bindTestDB(b, f).WithBindConfig(BindConfig{
				Binder: NewSliceRowsBinder[[]performanceRecord](), Capacity: max(n, 1),
			})
			builder := db.Select("*").From("records").Where(Eq("id", int64(42))).Limit(int64(max(n, 1)))
			compiled := mustCompileTemplate(b, builder.Clone().ClearWhere().Where(Eq("id", Param(0))))
			ctx := context.Background()
			for _, mode := range []string{"builder", "template"} {
				b.Run(mode, func(b *testing.B) {
					b.ReportAllocs()
					for b.Loop() {
						var rows *Rows
						if mode == "builder" {
							rows = builder.QueryRowsContext(ctx)
						} else {
							rows = compiled.QueryRowsContext(ctx, db, int64(42))
						}

						var got []performanceRecord
						if err := rows.Bind(&got); err != nil || len(got) != n {
							b.Fatal(len(got), err)
						}
					}
				})
			}
		})
	}
}
