// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"strconv"
	"testing"

	"github.com/xgfone/go-sqlx/dialect"
)

var expressionWorkloadNode any
var expressionWorkloadArgs []any

// Capture construction separately from repeated rendering: shrinking a handle
// is useful only when the complete escaped node and its payload also improve.
func BenchmarkExpressionConstruct(b *testing.B) {
	for _, tc := range []struct {
		name string
		make func() any
	}{
		{"Eq", func() any { return Eq("id", 7) }},
		{"EqExpr", func() any { return Eq(Ident("id"), 7) }},
		{"Ident", func() any { return Ident("id") }},
		{"IdentParts", func() any { return Ident("t", "id") }},
		{"Raw", func() any { return Expr("CURRENT_TIMESTAMP") }},
		{"Template", func() any { return Expr("? + ?", Ident("id"), 7) }},
		{"Value", func() any { return Value(7) }},
		{"Param", func() any { return Param(0) }},
		{"Count", func() any { return Count("id") }},
		{"Function", func() any { return Func("COALESCE", Ident("id"), 7) }},
		{"Case", func() any { return Case().When(Eq("id", 7), 1).Else(0).End() }},
		{"Window", func() any { return SumExpr(Ident("n")).Filter(Gt("n", 0)).Over(Window().PartitionBy("team")) }},
	} {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				expressionWorkloadNode = tc.make()
			}
		})
	}
}

func BenchmarkExpressionWorkloads(b *testing.B) {
	values := make([]any, 128)
	for i := range values {
		values[i] = i
	}

	for _, n := range []int{0, 1, 20, 100, 1000} {
		b.Run("conditions"+strconv.Itoa(n), func(b *testing.B) {
			makeQuery := func() *SelectBuilder {
				q := Select("id").From("t").SetDialect(dialect.Postgres)
				for i := range n {
					q.Where(Eq("id", i))
				}
				return q
			}

			b.Run("construct_build", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					q, a, e := makeQuery().Build()
					if e != nil {
						b.Fatal(e)
					}
					expressionHelperSQL = q
					expressionWorkloadArgs = a
				}
			})

			q := makeQuery()
			b.Run("build", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					s, a, e := q.Build()
					if e != nil {
						b.Fatal(e)
					}
					expressionHelperSQL = s
					expressionWorkloadArgs = a
				}
			})
		})
	}

	for _, tc := range []struct {
		name string
		make func() *SelectBuilder
	}{
		{"in128", func() *SelectBuilder {
			return Select("id").From("t").Where(In("id", values...))
		}},
		{"custom", func() *SelectBuilder {
			return Select("id").From("t").Where(ConditionWriterFunc(func(w *SQLWriter) (bool, error) {
				w.Path("id")
				w.Raw("=")
				w.Arg(7)
				return true, nil
			}))
		}},
		{"custom_group", func() *SelectBuilder {
			return Select("id").From("t").Where(Or(
				ConditionWriterFunc(func(*SQLWriter) (bool, error) { return false, nil }),
				ConditionWriterFunc(func(w *SQLWriter) (bool, error) {
					w.Path("id")
					w.Raw("=")
					w.Arg(7)
					return true, nil
				}),
				Eq("v", 8),
			))
		}},
	} {
		b.Run(tc.name, func(b *testing.B) {
			q := tc.make().SetDialect(dialect.Postgres)
			b.ReportAllocs()
			for b.Loop() {
				s, a, e := q.Build()
				if e != nil {
					b.Fatal(e)
				}
				expressionHelperSQL = s
				expressionWorkloadArgs = a
			}
		})
	}
}

func BenchmarkDistinctOnExpressions(b *testing.B) {
	custom := Expr("?", Case().When(Eq("id", 1), 2).Else(3).End())
	for _, tc := range []struct {
		name  string
		key   Expression
		order Expression
	}{
		{"identifier", Ident("id"), Ident("id")},
		{"qualified", Ident("t", "id"), Ident("t", "id")},
		{"template", Expr("? + ?", Ident("id"), 1), Expr("? + ?", Ident("id"), 1)},
		{"custom_reuse", custom, custom},
	} {
		b.Run(tc.name, func(b *testing.B) {
			q := Select("id").From("t").DistinctOnExpr(tc.key).
				OrderByExpr(tc.order, Asc).SetDialect(dialect.Postgres)
			b.ReportAllocs()
			for b.Loop() {
				s, args, err := q.Build()
				if err != nil {
					b.Fatal(err)
				}
				expressionHelperSQL, expressionWorkloadArgs = s, args
			}
		})
	}
}

func BenchmarkConflictExpressionTargets(b *testing.B) {
	for _, tc := range []struct {
		name string
		key  Expression
	}{
		{"column", Ident("id")},
		{"qualified", Ident("t", "id")},
		{"function", Func("lower", Ident("name"))},
	} {
		b.Run(tc.name, func(b *testing.B) {
			q := Insert().Into("t").Columns("id", "name").Values(1, "a").
				OnConflict(ConflictExpressions(tc.key).DoNothing()).SetDialect(dialect.Postgres)
			benchmarkSafetySQL(b, q)
		})
	}
}
