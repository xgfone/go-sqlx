// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/xgfone/go-sqlx/dialect"
)

func BenchmarkSyntaxBuild(b *testing.B) {
	values := make([]any, 128)
	columns := make([]string, 128)
	for i := range values {
		values[i] = i
		columns[i] = "column_" + strconv.Itoa(i)
	}

	for _, variant := range []struct {
		name    string
		dialect Dialect
	}{
		{"postgres/builtin", dialect.Postgres},
		{"postgres/configured", dialect.WithVersion(dialect.Postgres, 14, 0, 0)},
		{"sqlite3", dialect.SQLite},
	} {
		b.Run(variant.name, func(b *testing.B) {
			d := variant.dialect
			for _, tc := range []struct {
				name string
				q    SQLBuilder
			}{
				{
					"Function128",
					Select().SelectExpr(Func("COALESCE", values...)).SetDialect(d),
				},
				{
					"Window",
					Select().SelectExpr(SumExpr(Expr("? * ?", Ident("v"), 2)).Filter(Gt("v", 0)).
						Over(Window().PartitionBy("team").OrderBy("id", Asc))).From("t").SetDialect(d)},
				{
					"Values128",
					Insert().Into("t").Columns(columns...).Values(values...).SetDialect(d),
				},
				{
					"Conflict",
					Insert().Into("t").Columns("id", "v").Values(1, 2).
						OnConflict(ConflictColumns("id").DoUpdate(Set("v", Excluded("v"))).Where(Gt("t.v", 0))).
						SetDialect(d),
				},
				{
					"SourceJoin",
					Select("a.id").FromSource(ValuesSource("a", []string{"id"}, []any{1}, []any{2})).
						JoinSourceUsing(LeftJoin, TableSource("t", "b"), "id").SetDialect(d)},
				{
					"CTE",
					Select("id").With("q", Select("id").From("t").Where(Gt("v", 1))).From("q").SetDialect(d),
				},
			} {
				// Compare wrapper overhead for many arguments and nested statement scope.
				if variant.name == "postgres/configured" && tc.name != "Function128" && tc.name != "CTE" {
					continue
				}

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

// Keep both the common path and the paths requiring expression validation or
// reuse visible. Setup is outside timing; [SQLBuilder.Build] includes its owned SQL/args.
func BenchmarkSQLFixes(b *testing.B) {
	for _, d := range []Dialect{dialect.Postgres, dialect.MySQL, dialect.SQLite} {
		b.Run(d.Name(), func(b *testing.B) {
			e := Coalesce(Ident("v"), 0)
			cases := []struct {
				name string
				q    SQLBuilder
			}{
				{
					"select",
					Select("id").From("t").Where(Eq("id", 1)).SetDialect(d),
				},
				{
					"expression_where",
					Select("id").From("t").Where(Gt(e, 1)).SetDialect(d),
				},
				{
					"window",
					Select().SelectExpr(SumExpr(Expr("? * ?", Ident("v"), 2)).
						Over(Window().PartitionBy("team").OrderBy("id", Asc))).
						From("t").SetDialect(d),
				},
				{
					"group_reuse",
					Select().SelectExpr(e, Count("*")).From("t").GroupByExpr(e).
						OrderByExpr(e, Asc).SetDialect(d)},
				{
					"distinct_reuse",
					Select().SelectExpr(e).From("t").Distinct().
						OrderByExpr(e, Asc).SetDialect(d),
				},
			}

			if d != dialect.MySQL {
				cases = append(cases, struct {
					name string
					q    SQLBuilder
				}{
					"returning",
					Update().Table("t").Set(Set("v", 1)).
						ReturningExpr(Coalesce(Ident("t", "v"), 0), "v").
						SetDialect(d),
				})
			}

			for _, tc := range cases {
				b.Run(tc.name, func(b *testing.B) { benchmarkSafetySQL(b, tc.q) })
			}

			for _, n := range []int{1, 128, 1000} {
				b.Run(fmt.Sprintf("values_%d", n), func(b *testing.B) {
					rows := make([][]any, n)
					for i := range rows {
						rows[i] = []any{i, "label"}
					}
					benchmarkSafetySQL(b, Select("d.id").FromSource(ValuesSource("d", []string{"id", "label"}, rows...)).SetDialect(d))
				})
			}

			for _, n := range []int{16, 128} {
				b.Run(fmt.Sprintf("ordered_expressions_%d", n), func(b *testing.B) {
					q := Select().From("t").SetDialect(d)
					for i := range n {
						q.SelectExprAlias(Coalesce(Ident(fmt.Sprintf("c%d", i)), i), fmt.Sprintf("a%d", i))
					}
					benchmarkSafetySQL(b, q.OrderByAsc("a0"))
				})
			}
		})
	}
}
