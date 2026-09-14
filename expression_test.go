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
