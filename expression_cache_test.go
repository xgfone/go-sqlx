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
