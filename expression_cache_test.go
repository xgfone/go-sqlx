// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/xgfone/go-sqlx/dialect"
)

func TestExpressionCacheLargeQueries(t *testing.T) {
	for _, templates := range []bool{false, true} {
		q := Select().From("t").SetDialect(dialect.Postgres)
		var wantArgs []any
		for i := range 24 {
			column := fmt.Sprintf("v%d", i)
			e := Coalesce(Ident(column), i)
			if templates {
				e = Expr("COALESCE(?, ?)", Ident(column), i)
			}

			q.SelectExpr(e)
			if templates {
				// Independent templates must still match after the identity
				// index is enabled, via structural equivalence.
				e = Expr("COALESCE(?, ?)", Ident(column), i)
			}

			q.GroupByExpr(e)
			wantArgs = append(wantArgs, i)
		}

		first, args, err := q.Build()
		if err != nil || !reflect.DeepEqual(args, wantArgs) {
			t.Fatalf("%s: %v, %v", first, args, err)
		}

		for i := range 24 {
			fragment := fmt.Sprintf(`COALESCE("v%d", $%d)`, i, i+1)
			if strings.Count(first, fragment) != 2 {
				t.Fatalf("parameter identity lost: %s", first)
			}
		}

		// Subsequent pool reuse must not alter a returned SQL string or args.
		_, _, err = q.Clone().Where(Eq("x", 99)).Build()
		if err != nil {
			t.Fatal(err)
		}

		again, _, err := q.Build()
		if err != nil || first != again || !reflect.DeepEqual(args, wantArgs) {
			t.Fatal("cache reuse altered a built query", err)
		}
	}
}

func TestExpressionCacheReleasesReferences(t *testing.T) {
	for _, count := range []int{12, maxPooledExpressions + 1} {
		cache := expressionCachePool.Get().(*expressionCache)
		for i := range count {
			cache.add(Value(&struct{ value int }{i}), fmt.Sprintf("$%d", i+1))
		}

		values, index := cache.values, cache.index
		releaseExpressionCache(cache)
		for _, v := range values {
			if v.expression.node != nil || v.expression.sql != "" || v.sql != "" {
				t.Fatal("cache retained an expression or SQL string")
			}
		}

		if len(index) != 0 {
			t.Fatal("cache retained identity keys")
		}
		if count > maxPooledExpressions && (len(cache.values) != 0 ||
			cap(cache.values) != smallExpressionCache || cache.index != nil) {
			t.Fatal("oversized cache storage retained")
		}
	}
}

func TestExpressionCacheGroupingAliases(t *testing.T) {
	e := Coalesce(Ident("v"), 0)
	checkSQL(t,
		Select().SelectExprAlias(e, "a").SelectExprAlias(e, "b").From("t").
			GroupBy("a").Having(Gt(e, 0)).OrderByAsc("a").SetDialect(dialect.Postgres),
		`SELECT COALESCE("v", $1) AS "a", COALESCE("v", $1) AS "b" FROM "t" GROUP BY "a" HAVING (COALESCE("v", $1) > $2) ORDER BY "a" ASC`,
		0, 0)
}

func TestRepeatedExpressionParameters(t *testing.T) {
	e := Coalesce(Ident("v"), 0)
	q := Select().SelectExpr(e, Count("*")).From("t").Where(Gt("v", -1)).
		GroupByExpr(e).Having(Gt(e, 0)).OrderByExpr(e, Asc)

	for _, d := range []Dialect{dialect.Postgres, dialect.WithVersion(dialect.Postgres, 14, 0, 0)} {
		checkSQL(t, q.Clone().SetDialect(d),
			`SELECT COALESCE("v", $1), COUNT(*) FROM "t" WHERE ("v" > $2) GROUP BY COALESCE("v", $1) HAVING (COALESCE("v", $1) > $3) ORDER BY COALESCE("v", $1) ASC`, 0, -1, 0)
		checkSQL(t, Select().SelectExpr(e).From("t").Distinct().OrderByExpr(e, Desc).SetDialect(d),
			`SELECT DISTINCT COALESCE("v", $1) FROM "t" ORDER BY COALESCE("v", $1) DESC`, 0)
		checkSQL(t, Select().SelectExpr(Cast(e, "TEXT"), Count("*")).From("t").
			GroupByExpr(GroupingSets(GroupingSet(e), GroupingSet())).SetDialect(d),
			`SELECT CAST(COALESCE("v", $1) AS TEXT), COUNT(*) FROM "t" GROUP BY GROUPING SETS ((COALESCE("v", $1)), ())`, 0)
	}

	checkSQL(t, q.Clone().SetDialect(dialect.SQLite),
		`SELECT COALESCE("v", ?), COUNT(*) FROM "t" WHERE ("v" > ?) GROUP BY COALESCE("v", ?) HAVING (COALESCE("v", ?) > ?) ORDER BY COALESCE("v", ?) ASC`, 0, -1, 0, 0, 0, 0)
	checkSQL(t, q.Clone().SetDialect(dialect.MySQL),
		"SELECT COALESCE(`v`, ?), COUNT(*) FROM `t` WHERE (`v` > ?) GROUP BY COALESCE(`v`, ?) HAVING (COALESCE(`v`, ?) > ?) ORDER BY COALESCE(`v`, ?) ASC", 0, -1, 0, 0, 0, 0)

	// Equal constants in different expressions do not imply expression identity.
	other := Coalesce(Ident("w"), 0)
	checkSQL(t, Select().SelectExpr(e, other).From("t").GroupByExpr(e, other).SetDialect(dialect.Postgres),
		`SELECT COALESCE("v", $1), COALESCE("w", $2) FROM "t" GROUP BY COALESCE("v", $1), COALESCE("w", $2)`, 0, 0)
	// The same expression in a subquery gets that query's own bindings.
	sub := Select().SelectExpr(e).From("other").Distinct().OrderByExpr(e, Asc).Limit(1)
	checkSQL(t, Select().SelectExpr(e, Subquery(sub)).From("t").GroupByExpr(e).SetDialect(dialect.Postgres),
		`SELECT COALESCE("v", $1), (SELECT DISTINCT COALESCE("v", $2) FROM "other" ORDER BY COALESCE("v", $2) ASC LIMIT 1) FROM "t" GROUP BY COALESCE("v", $1)`, 0, 0)

	e = Coalesce(Ident("v"), Param(0))
	tmpl, err := Select().SelectExpr(e, Count("*")).From("t").GroupByExpr(e).
		Having(Gt(e, Param(1))).SetDialect(dialect.Postgres).Compile()
	if err != nil {
		t.Fatal(err)
	}

	for _, fallback := range []int{0, 7} {
		query, args, err := tmpl.Bind(fallback, 1)
		if err != nil || query != `SELECT COALESCE("v", $1), COUNT(*) FROM "t" GROUP BY COALESCE("v", $1) HAVING (COALESCE("v", $1) > $2)` || !reflect.DeepEqual(args, []any{fallback, 1}) {
			t.Fatalf("compiled reuse: %q %#v %v", query, args, err)
		}
	}

	// Sharing a runtime input across different type contexts must not unify its
	// placeholders just because both expressions contain the same Param.
	p := Param(0)
	left, right := Coalesce(Ident("n"), p), Coalesce(Ident("s"), p)
	tmpl, err = Select().SelectExprAlias(left, "a").SelectExprAlias(right, "b").From("t").
		Distinct().OrderByAsc("a").SetDialect(dialect.Postgres).Compile()
	if err != nil {
		t.Fatal(err)
	}

	query, args, err := tmpl.Bind("2")
	if err != nil || query != `SELECT DISTINCT COALESCE("n", $1) AS "a", COALESCE("s", $2) AS "b" FROM "t" ORDER BY "a" ASC` || !reflect.DeepEqual(args, []any{"2", "2"}) {
		t.Fatalf("independent parameter type contexts: %q %#v %v", query, args, err)
	}

	v := Value("2")
	checkSQL(t, Select().SelectExpr(Coalesce(Ident("n"), v), Coalesce(Ident("s"), v)).From("t").Distinct().OrderByExpr(Expr("1"), Asc).SetDialect(dialect.Postgres),
		`SELECT DISTINCT COALESCE("n", $1), COALESCE("s", $2) FROM "t" ORDER BY 1 ASC`, "2", "2")
	checkSQL(t, Select().SelectExpr(v).Distinct().OrderByExpr(v, Asc).SetDialect(dialect.Postgres),
		`SELECT DISTINCT $1 ORDER BY $1 ASC`, "2")
}
