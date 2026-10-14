// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"database/sql"
	"reflect"
	"strings"
	"testing"

	"github.com/xgfone/go-sqlx/dialect"
)

// Keep all data/expression pairs under the same binding contract. InSlice and
// NotInSlice use the variadic expression entry points for their counterparts.
func TestTypedColumnDataExpressionBinding(t *testing.T) {
	c := NewTypedColumn[any]("v")
	for _, d := range []Dialect{dialect.MySQL, dialect.Postgres, dialect.SQLite} {
		t.Run(d.Name(), func(t *testing.T) {
			update := func(u Updater) templateBuilder { return Update().Table("t").Set(u).SetDialect(d) }
			where := func(c Condition) templateBuilder {
				return Select("v").From("t").Where(c).SetDialect(d)
			}
			selectExpr := func(e Expression) templateBuilder {
				return Select().SelectExpr(e).From("t").SetDialect(d)
			}
			cases := []struct {
				name  string
				data  func(any) templateBuilder
				expr  func(Expression) templateBuilder
				count int
			}{
				{
					"Set",
					func(v any) templateBuilder { return update(c.Set(v)) },
					func(e Expression) templateBuilder { return update(c.SetExpr(e)) },
					1,
				},
				{
					"ColValue",
					func(v any) templateBuilder { return Insert().Into("t").Row(c.ColValue(v)).SetDialect(d) },
					func(e Expression) templateBuilder { return Insert().Into("t").Row(c.ColValueExpr(e)).SetDialect(d) },
					1,
				},
				{
					"Eq",
					func(v any) templateBuilder { return where(c.Eq(v)) },
					func(e Expression) templateBuilder { return where(c.EqExpr(e)) },
					1,
				},
				{
					"Ne",
					func(v any) templateBuilder { return where(c.Ne(v)) },
					func(e Expression) templateBuilder { return where(c.NeExpr(e)) },
					1,
				},
				{
					"Gt",
					func(v any) templateBuilder { return where(c.Gt(v)) },
					func(e Expression) templateBuilder { return where(c.GtExpr(e)) },
					1,
				},
				{
					"Ge",
					func(v any) templateBuilder { return where(c.Ge(v)) },
					func(e Expression) templateBuilder { return where(c.GeExpr(e)) },
					1,
				},
				{
					"Lt",
					func(v any) templateBuilder { return where(c.Lt(v)) },
					func(e Expression) templateBuilder { return where(c.LtExpr(e)) },
					1,
				},
				{
					"Le",
					func(v any) templateBuilder { return where(c.Le(v)) },
					func(e Expression) templateBuilder { return where(c.LeExpr(e)) },
					1,
				},
				{
					"Between",
					func(v any) templateBuilder { return where(c.Between(v, v)) },
					func(e Expression) templateBuilder { return where(c.BetweenExpr(e, e)) },
					2,
				},
				{
					"NotBetween",
					func(v any) templateBuilder { return where(c.NotBetween(v, v)) },
					func(e Expression) templateBuilder { return where(c.NotBetweenExpr(e, e)) },
					2,
				},
				{
					"In",
					func(v any) templateBuilder { return where(c.In(v)) },
					func(e Expression) templateBuilder { return where(c.InExpr(e)) },
					1,
				},
				{
					"NotIn",
					func(v any) templateBuilder { return where(c.NotIn(v)) },
					func(e Expression) templateBuilder { return where(c.NotInExpr(e)) },
					1,
				},
				{
					"InSlice",
					func(v any) templateBuilder { return where(c.InSlice([]any{v})) },
					func(e Expression) templateBuilder { return where(c.InExpr(e)) },
					1,
				},
				{
					"NotInSlice", func(v any) templateBuilder { return where(c.NotInSlice([]any{v})) },
					func(e Expression) templateBuilder { return where(c.NotInExpr(e)) },
					1,
				},
				{
					"Like",
					func(v any) templateBuilder { return where(c.Like(v)) },
					func(e Expression) templateBuilder { return where(c.LikeExpr(e)) },
					1,
				},
				{
					"NotLike",
					func(v any) templateBuilder { return where(c.NotLike(v)) },
					func(e Expression) templateBuilder { return where(c.NotLikeExpr(e)) },
					1,
				},
				{
					"Add",
					func(v any) templateBuilder { return update(c.Add(v)) },
					func(e Expression) templateBuilder { return update(c.AddExpr(e)) },
					1,
				},
				{
					"Sub",
					func(v any) templateBuilder { return update(c.Sub(v)) },
					func(e Expression) templateBuilder { return update(c.SubExpr(e)) },
					1,
				},
				{
					"Mul",
					func(v any) templateBuilder { return update(c.Mul(v)) },
					func(e Expression) templateBuilder { return update(c.MulExpr(e)) },
					1,
				},
				{
					"Div",
					func(v any) templateBuilder { return update(c.Div(v)) },
					func(e Expression) templateBuilder { return update(c.DivExpr(e)) },
					1,
				},
				{
					"Coalesce", func(v any) templateBuilder { return selectExpr(c.Coalesce(v)) },
					func(e Expression) templateBuilder { return selectExpr(c.CoalesceExpr(e)) },
					1,
				},
				{
					"NullIf",
					func(v any) templateBuilder { return selectExpr(c.NullIf(v)) },
					func(e Expression) templateBuilder { return selectExpr(c.NullIfExpr(e)) },
					1,
				},
			}

			// Check both render paths, including constants retained by a template.
			checkBound := func(t *testing.T, b templateBuilder, want []any) string {
				t.Helper()

				query, args, err := b.Build()
				if err != nil || !reflect.DeepEqual(args, want) {
					t.Fatalf("Build got %q %#v %v; want args %#v", query, args, err, want)
				}

				compiledSQL, compiledArgs, err := mustCompileTemplate(t, b).Bind()
				if err != nil || compiledSQL != query || !reflect.DeepEqual(compiledArgs, want) {
					t.Fatalf("Compile/Bind got %q %#v %v; want %q %#v", compiledSQL, compiledArgs, err, query, want)
				}

				return query
			}

			marker := Expr("typed_binding_marker()")
			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					repeat := func(value any) []any {
						args := make([]any, tc.count)
						for i := range args {
							args[i] = value
						}
						return args
					}

					dataSQL := checkBound(t, tc.data(1), repeat(1))
					exprSQL := checkBound(t, tc.expr(Value(1)), repeat(1))
					for _, value := range []struct {
						name string
						data Expression
					}{
						{"expression", marker},
						{"value", Value(1)},
						{"value_null", Value(nil)},
						{"value_param", Value(Param(0))},
					} {
						t.Run("data/"+value.name, func(t *testing.T) {
							if query := checkBound(t, tc.data(value.data), repeat(value.data)); query != dataSQL {
								t.Fatalf("data changed SQL: got %q; want %q", query, dataSQL)
							}
						})
					}

					t.Run("expr/sql", func(t *testing.T) {
						query := checkBound(t, tc.expr(marker), nil)
						if strings.Count(query, "typed_binding_marker()") != tc.count {
							t.Fatalf("expression was not rendered at every operand: %q", query)
						}
					})

					t.Run("expr/value_null", func(t *testing.T) {
						if query := checkBound(t, tc.expr(Value(nil)), repeat(nil)); query != exprSQL {
							t.Fatalf("bound NULL changed SQL: got %q; want %q", query, exprSQL)
						}
					})

					for _, slot := range []struct {
						name string
						b    templateBuilder
						sql  string
					}{
						{"data", tc.data(Param(0)), dataSQL},
						// Direct Param is scalar; arithmetic does not add the
						// parentheses used for other Expression operands.
						{"expr", tc.expr(Param(0)), dataSQL},
						{"expr_value", tc.expr(Value(Param(0))), exprSQL},
					} {
						t.Run("param/"+slot.name, func(t *testing.T) {
							checkBuildError(t, slot.b)
							compiled := mustCompileTemplate(t, slot.b)
							for _, input := range []any{"runtime value", nil} {
								query, args, err := compiled.Bind(input)
								if err != nil || query != slot.sql ||
									!reflect.DeepEqual(args, repeat(input)) {
									t.Fatalf("Bind got %q %#v %v; want %q %#v",
										query, args, err, slot.sql, repeat(input))
								}
							}
						})
					}
				})
			}
		})
	}
}

func TestTypedColumnSetNamedBinding(t *testing.T) {
	c := NewTypedColumn[any]("v")
	for _, d := range []Dialect{dialect.MySQL, dialect.Postgres, dialect.SQLite} {
		t.Run(d.Name(), func(t *testing.T) {
			for _, value := range []struct {
				name string
				data any
			}{
				{"scalar", 1},
				{"null", nil},
				{"expression", Expr("typed_binding_marker()")},
				{"value", Value(1)},
				{"value_null", Value(nil)},
				{"value_param", Value(Param(0))},
			} {
				t.Run(value.name, func(t *testing.T) {
					for _, name := range []string{"n", ""} {
						b := Update().Table("t").Set(c.SetNamed(name, value.data)).SetDialect(d)
						want := []any{value.data}
						placeholder := d.Placeholder(1)
						if d.Name() == dialect.SQLite.Name() && name != "" {
							want = []any{sql.Named(name, value.data)}
							placeholder = "@" + name
						}

						wantSQL := "UPDATE " + d.QuoteIdent("t") + " SET " + d.QuoteIdent("v") + "=" + placeholder
						checkSQL(t, b, wantSQL, want...)
						query, args, err := mustCompileTemplate(t, b).Bind()
						if err != nil || query != wantSQL || !reflect.DeepEqual(args, want) {
							t.Fatalf("named Compile/Bind got %q %#v %v; want %q %#v", query, args, err, wantSQL, want)
						}
					}
				})
			}

			for _, name := range []string{"n", ""} {
				for _, updater := range []Updater{
					c.SetNamed(name, Param(0)),
					NewTypedColumn[Expression]("v").SetNamed(name, Param(0)),
				} {
					b := Update().Table("t").Set(updater).SetDialect(d)
					checkBuildError(t, b)
					if _, err := b.Compile(); err == nil ||
						!strings.Contains(err.Error(), "sqlx.Param inside sql.Named is not supported") {
						t.Fatalf("expected named Param error with name %q; got %v", name, err)
					}
				}
			}
		})
	}
}
