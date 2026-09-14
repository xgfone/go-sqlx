// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"testing"

	"github.com/xgfone/go-sqlx/dialect"
)

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

func TestDistinctOnCompoundOrdering(t *testing.T) {
	for _, set := range []struct {
		name  string
		apply func(*SelectBuilder, *SelectBuilder) *SelectBuilder
	}{
		{"UNION", (*SelectBuilder).Union},
		{"UNION ALL", (*SelectBuilder).UnionAll},
		{"INTERSECT", (*SelectBuilder).Intersect},
		{"EXCEPT", (*SelectBuilder).Except},
	} {
		t.Run(set.name, func(t *testing.T) {
			e := Coalesce(Ident("a"), 0)
			first := Select().SelectExprAlias(e, "key").From("t").DistinctOnExpr(e)
			q := set.apply(first, Select("a").From("u")).SetDialect(dialect.Postgres)
			prefix := `SELECT DISTINCT ON (COALESCE("a", $1)) COALESCE("a", $1) AS "key" FROM "t" ` + set.name + ` SELECT "a" FROM "u" ORDER BY `
			checkSQL(t, q.Clone().OrderByAsc("key"), prefix+`"key" ASC`, 0)
			checkSQL(t, q.Clone().OrderByExpr(Expr("1"), Desc), prefix+`1 DESC`, 0)
			// Global ordering need not start with the first operand's DISTINCT key.
			checkSQL(t, set.apply(Select("a", "b").From("t").DistinctOn("a"), Select("a", "b").From("u")).
				OrderByAsc("b").SetDialect(dialect.Postgres),
				`SELECT DISTINCT ON ("a") "a", "b" FROM "t" `+set.name+` SELECT "a", "b" FROM "u" ORDER BY "b" ASC`)
			checkBuildError(t, q.Clone().Distinct())
		})
	}
}

func TestComputedDistinctOnReusesParameters(t *testing.T) {
	e := Coalesce(Ident("name"), "unknown")
	b := Select("id").From("t").DistinctOnExpr(e).OrderByExpr(e, Asc).
		Where(Eq("active", true)).SetDialect(dialect.Postgres)
	checkSQL(t, b,
		`SELECT DISTINCT ON (COALESCE("name", $1)) "id" FROM "t" WHERE ("active" = $2) ORDER BY COALESCE("name", $1) ASC`,
		"unknown", true)

	e = Expr("COALESCE(?, ?)", Ident("name"), "fallback")
	checkSQL(t,
		Select("id").From("t").DistinctOnExpr(e).OrderByExpr(e, Desc).SetDialect(dialect.Postgres),
		`SELECT DISTINCT ON (COALESCE("name", $1)) "id" FROM "t" ORDER BY COALESCE("name", $1) DESC`,
		"fallback")
	checkSQL(t,
		Select().SelectExprAlias(e, "key").From("t").DistinctOnExpr(e).OrderByAsc("key").SetDialect(dialect.Postgres),
		`SELECT DISTINCT ON (COALESCE("name", $1)) COALESCE("name", $1) AS "key" FROM "t" ORDER BY "key" ASC`,
		"fallback")
	checkSQL(t,
		Select("id").From("t").DistinctOn("id", "id").OrderByAsc("id").OrderByAsc("v").SetDialect(dialect.Postgres),
		`SELECT DISTINCT ON ("id", "id") "id" FROM "t" ORDER BY "id" ASC, "v" ASC`)
	checkSQL(t,
		Select("id").From("t").DistinctOn("id").OrderByExpr(Expr("1"), Asc).SetDialect(dialect.Postgres),
		`SELECT DISTINCT ON ("id") "id" FROM "t" ORDER BY 1 ASC`)
	checkSQL(t,
		Select().SelectExprAlias(e, "key").From("t").DistinctOnExpr(e, e).
			OrderByAsc("key").SetDialect(dialect.Postgres),
		`SELECT DISTINCT ON (COALESCE("name", $1), COALESCE("name", $1)) COALESCE("name", $1) AS "key" FROM "t" ORDER BY "key" ASC`,
		"fallback")
}
