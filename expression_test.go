// Copyright 2026 xgfone
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package sqlx

import (
	"strings"
	"testing"

	"github.com/xgfone/go-sqlx/dialect"
)

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
		{"package", SelectExpr(Count("id")).From("users").SetDB(db), false},
		{"package_alias", SelectExprAlias(Count("id"), "total.count").From("users").SetDB(db), true},
		{"db", db.SelectExpr(Count("id")).From("users"), false},
		{"db_alias", db.SelectExprAlias(Count("id"), "total.count").From("users"), true},
		{"table", table.SelectExpr(Count("id")), false},
		{"table_alias", table.SelectExprAlias(Count("id"), "total.count"), true},
		{"builder", db.SelectBuilder().From("users").SelectExpr(Count("id")), false},
		{"builder_alias", db.SelectBuilder().From("users").SelectExprAlias(Count("id"), "total.count"), true},
	} {
		t.Run(test.name, func(t *testing.T) {
			want := `SELECT COUNT("id")`
			if test.alias {
				want += ` AS "total.count"`
			}
			want += ` FROM "users"`
			if got, args := test.builder.Build(); got != want || len(args) != 0 {
				t.Fatalf("got %q %#v, want %q", got, args, want)
			}
		})
	}
}

var expressionResult string

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
