// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"reflect"
	"strings"
	"testing"

	"github.com/xgfone/go-sqlx/dialect"
)

func TestExpressionLayout(t *testing.T) {
	typ := reflect.TypeFor[Expression]()
	want := reflect.TypeFor[string]().Size() + reflect.TypeFor[any]().Size()
	if typ.Size() != want {
		t.Fatalf("Expression size = %d, want %d; check field padding", typ.Size(), want)
	}
	if typ.Comparable() {
		t.Fatal("Expression must remain non-comparable")
	}
}

func TestExpressionLineCommentTerminators(t *testing.T) {
	for _, d := range []Dialect{dialect.Postgres, dialect.SQLite, dialect.MySQL} {
		t.Run(d.Name(), func(t *testing.T) {
			for _, ending := range []string{"\n", "\r\n"} {
				e := Expr("? -- ignored ?"+ending+" + ?", 1, 2)
				checkSQL(t, Select().SelectExpr(e).SetDialect(d),
					"SELECT "+d.Placeholder(1)+" -- ignored ?"+ending+" + "+d.Placeholder(2), 1, 2)
			}
			if d == dialect.Postgres {
				checkSQL(t, Select().SelectExpr(Expr("? -- ignored ?\r + ?", 1, 2)).SetDialect(d),
					"SELECT $1 -- ignored ?\r + $2", 1, 2)
			} else {
				checkSQL(t, Select().SelectExpr(Expr("? -- ignored ?\r + ?", 1)).SetDialect(d),
					"SELECT ? -- ignored ?\r + ?", 1)
			}
		})
	}

	// The configurable rule, rather than the dialect's name, controls scanning.
	rules := dialect.Postgres.LexicalRules()
	rules.LineCommentCR = false
	checkSQL(t, Select().SelectExpr(Expr("? -- ignored ?\r + ?", 1)).
		SetDialect(dialect.WithLexicalRules(dialect.Postgres, rules)),
		"SELECT $1 -- ignored ?\r + ?", 1)

	rules = dialect.SQLite.LexicalRules()
	rules.LineCommentCR = true
	checkSQL(t, Select().SelectExpr(Expr("? -- ignored ?\r + ?", 1, 2)).
		SetDialect(dialect.WithLexicalRules(dialect.SQLite, rules)),
		"SELECT ? -- ignored ?\r + ?", 1, 2)
}

func TestExpressionBuild(t *testing.T) {
	for _, test := range []struct {
		expr Expression
		want string
	}{
		{Expr("COALESCE(a, b)"), "COALESCE(a, b)"},
		{Ident("literal.dot"), `"literal.dot"`},
		{Ident("schema", `ta"ble`, "id"), `"schema"."ta""ble"."id"`},
		{Ident("*"), `"*"`},
		{Ident("table", "*"), `"table"."*"`},
		{Count("*"), "COUNT(*)"},
		{Count("table.*"), `COUNT("table".*)`},
		{Count("id"), `COUNT("id")`},
		{CountDistinct("table.id"), `COUNT(DISTINCT "table"."id")`},
		{Sum("table.price"), `SUM("table"."price")`},
	} {
		if got := test.expr.build(dialect.Postgres); got != test.want {
			t.Errorf("%s: got %q, want %q", test.expr.String(), got, test.want)
		}
	}
	// Quoted output can exceed the initial size estimate, including for custom dialects.
	d := verboseQuoteDialect{Dialect: dialect.Postgres}
	if got := CountDistinct("table.id").build(d); got != "COUNT(DISTINCT [identifier:table].[identifier:id])" {
		t.Fatal(got)
	}
	if got := Ident("ta]ble", "id").build(d); got != "[identifier:ta]]ble].[identifier:id]" {
		t.Fatal(got)
	}
}

type verboseQuoteDialect struct{ Dialect }

func (verboseQuoteDialect) QuoteIdent(name string) string {
	return "[identifier:" + strings.ReplaceAll(name, "]", "]]") + "]"
}

func TestTypedSelectExpressions(t *testing.T) {
	db := &DB{Dialect: dialect.Postgres}
	table := db.NewTable("users")
	for _, test := range []struct {
		name    string
		builder *SelectBuilder
		alias   bool
	}{
		{"package", Select().SelectExpr(Count("id")).From("users").SetDB(db), false},
		{"package_alias", Select().SelectExprAlias(Count("id"), "total.count").From("users").SetDB(db), true},
		{"db", db.Select().SelectExpr(Count("id")).From("users"), false},
		{"db_alias", db.Select().SelectExprAlias(Count("id"), "total.count").From("users"), true},
		{"table", table.Select().SelectExpr(Count("id")), false},
		{"table_alias", table.Select().SelectExprAlias(Count("id"), "total.count"), true},
		{"builder", db.Select().From("users").SelectExpr(Count("id")), false},
		{"builder_alias", db.Select().From("users").SelectExprAlias(Count("id"), "total.count"), true},
	} {
		t.Run(test.name, func(t *testing.T) {
			want := `SELECT COUNT("id")`
			if test.alias {
				want += ` AS "total.count"`
			}
			want += ` FROM "users"`
			if got, args := test.builder.MustBuild(); got != want || len(args) != 0 {
				t.Fatalf("got %q %#v, want %q", got, args, want)
			}
		})
	}
}

var expressionResult string

func TestFunctionNames(t *testing.T) {
	for _, d := range []Dialect{dialect.Postgres, dialect.MySQL, dialect.SQLite} {
		for _, name := range []string{"_fn", "public._fn", "函数"} {
			checkSQL(t, Select().SelectExpr(Func(name, 1)).SetDialect(d),
				"SELECT "+name+"("+d.Placeholder(1)+")", 1)
		}
		checkBuildError(t, Select().SelectExpr(Func("", 1)).SetDialect(d))
	}
}

func BenchmarkExpressionBuild(b *testing.B) {
	for _, test := range []struct {
		name string
		expr Expression
	}{
		{"raw", Expr("COALESCE(a, b)")},
		{"identifier", Ident("id")},
		{"qualified", Ident("schema", "users", "id")},
		{"count", Count("id")},
		{"count_path", Count("users.id")},
		{"distinct", CountDistinct("users.id")},
	} {
		b.Run(test.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				expressionResult = test.expr.build(dialect.Postgres)
			}
		})
	}
}

func TestDistinctOnNestedExpressionIdentity(t *testing.T) {
	for _, tc := range []struct {
		name string
		make func() Expression
	}{
		{"value", func() Expression { return Value(1) }},
		{"function", func() Expression { return Func("COALESCE", Ident("v"), 1) }},
		{"case", func() Expression { return Case().When(Eq("id", 1), 2).Else(3).End() }},
		{"tuple", func() Expression { return Tuple(Ident("a"), Ident("b")) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Reusing the complete description still reuses its parameters.
			left := Expr("?", tc.make())
			if _, _, err := Select("id").DistinctOnExpr(left).
				OrderByExpr(left, Asc).SetDialect(dialect.Postgres).Build(); err != nil {
				t.Fatal(err)
			}
		})
	}

	// Simple structured templates continue to compare by content.
	checkSQL(t, Select("id").DistinctOnExpr(Expr("? + ?", Ident("v"), 1)).
		OrderByExpr(Expr("? + ?", Ident("v"), 1), Asc).SetDialect(dialect.Postgres),
		`SELECT DISTINCT ON ("v" + $1) "id" ORDER BY "v" + $1 ASC`, 1)
}
