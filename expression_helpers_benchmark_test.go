// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"strconv"
	"testing"

	"github.com/xgfone/go-sqlx/dialect"
)

var expressionHelperSQL string

// Reuse the helper and argument storage to measure rendering allocations.
func benchmarkExpressionHelper(b *testing.B, d Dialect, render func(*BuildContext) string) {
	c := NewBuildContext(d)
	b.ReportAllocs()
	for b.Loop() {
		c.args = c.args[:0]
		expressionHelperSQL = render(c)
	}
}

func BenchmarkExpressionHelpers(b *testing.B) {
	for _, d := range []Dialect{dialect.Postgres, dialect.SQLite} {
		b.Run(d.Name(), func(b *testing.B) {
			b.Run("Between", func(b *testing.B) {
				benchmarkExpressionHelper(b, d, Between("score", 10, 100).BuildCondition)
			})

			for _, n := range []int{2, 16, 128} {
				values := make([]any, n)
				columns := make([]string, n)
				searched, simple := Case(), CaseValue(Ident("id"))
				for i := range n {
					values[i] = i
					columns[i] = "column_" + strconv.Itoa(i)
					searched.When(Eq("id", i), i+1)
					simple.WhenValue(i, i+1)
				}

				for _, helper := range []struct {
					name   string
					render func(*BuildContext) string
				}{
					{"In", In("id", values...).BuildCondition},
					{"Case", searched.Else(-1).End().render},
					{"CaseValue", simple.Else(-1).End().render},
					{"SetRow", SetRow(columns, values...).BuildUpdate},
				} {
					b.Run(helper.name+"/"+strconv.Itoa(n), func(b *testing.B) {
						benchmarkExpressionHelper(b, d, helper.render)
					})
				}
			}
		})
	}
}

func BenchmarkSyntaxBuild(b *testing.B) {
	values := make([]any, 128)
	columns := make([]string, 128)
	for i := range values {
		values[i] = i
		columns[i] = "column_" + strconv.Itoa(i)
	}
	for _, d := range []Dialect{dialect.Postgres, dialect.WithVersion(dialect.Postgres, 14, 0, 0), dialect.SQLite} {
		b.Run(d.Name(), func(b *testing.B) {
			for _, tc := range []struct {
				name string
				q    Statement
			}{
				{"Function128", Select().SelectExpr(Func("COALESCE", values...)).SetDialect(d)},
				{"Window", Select().SelectExpr(SumExpr(Expr("? * ?", Ident("v"), 2)).
					Filter(Gt("v", 0)).Over(Window().PartitionBy("team").OrderBy("id", Asc))).From("t").SetDialect(d)},
				{"Values128", Insert().Into("t").Columns(columns...).Values(values...).SetDialect(d)},
				{"Conflict", Insert().Into("t").Columns("id", "v").Values(1, 2).
					OnConflict(ConflictColumns("id").DoUpdate(Set("v", Excluded("v"))).Where(Gt("t.v", 0))).SetDialect(d)},
				{"SourceJoin", Select("a.id").FromSource(ValuesSource("a", []string{"id"}, []any{1}, []any{2})).
					JoinSourceUsing(LeftJoin, TableSource("t", "b"), "id").SetDialect(d)},
				{"CTE", Select("id").With("q", Select("id").From("t").Where(Gt("v", 1))).From("q").SetDialect(d)},
			} {
				b.Run(tc.name, func(b *testing.B) {
					b.ReportAllocs()
					for b.Loop() {
						q, _, err := tc.q.Build()
						if err != nil {
							b.Fatal(err)
						}
						expressionHelperSQL = q
					}
				})
			}
		})
	}
}

func BenchmarkMixedSetOperations(b *testing.B) {
	for _, n := range []int{16, 128} {
		q := Select().SelectExpr(Value(0)).SetDialect(dialect.Postgres)
		for i := 1; i < n; i++ {
			operand := Select().SelectExpr(Value(i))
			if i%2 == 0 {
				q.Intersect(operand)
			} else {
				q.Union(operand)
			}
		}
		b.Run(strconv.Itoa(n), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				s, _, err := q.Build()
				if err != nil {
					b.Fatal(err)
				}
				expressionHelperSQL = s
			}
		})
	}
}
