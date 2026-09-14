// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"context"
	"testing"

	"github.com/xgfone/go-sqlx/dialect"
)

func TestExplicitExpressionsAndIdentifiers(t *testing.T) {
	b := Select("u.*").SelectAlias("first name", "name.with.dot").
		Select(`a"b`).SelectExpr(Ident("literal.dot")).
		Select().SelectExpr(Expr("GREATEST(MAX(a),MIN(b))")).
		Select().SelectExpr(Count("u.id")).
		FromAlias("users", "u").
		SetDB(&DB{Dialect: dialect.Postgres})
	q, args := b.MustBuild()

	want := `SELECT "u".*, "first name" AS "name.with.dot", "a""b", "literal.dot", GREATEST(MAX(a),MIN(b)), COUNT("u"."id") FROM "users" AS "u"`
	if q != want || len(args) != 0 {
		t.Fatalf("got %s\nwant %s", q, want)
	}

	b = Select("COALESCE(a,b)").From("t").SetDB(&DB{Dialect: dialect.Postgres})
	if q, _ := b.MustBuild(); q != `SELECT "COALESCE(a,b)" FROM "t"` {
		t.Fatal(q)
	}

	b = Select("*").From("t").SelectExpr(Count("id")).SetDB(&DB{Dialect: dialect.Postgres})
	if q, _ := b.MustBuild(); q != `SELECT *, COUNT("id") FROM "t"` {
		t.Fatal(q)
	}
}

func TestExplicitSortingHavingAndPagination(t *testing.T) {
	checkSQL(t, Select("id").From("t").OrderByDesc("time"),
		"SELECT `id` FROM `t` ORDER BY `time` DESC")
	checkSQL(t, Select().SelectExpr(Count("*")).From("t").Having(Expr("COUNT(*) > ?", 2).Condition()),
		"SELECT COUNT(*) FROM `t` HAVING (COUNT(*) > ?)", 2)
	checkSQL(t, Select("a.id").FromAlias("t", "a").FromAlias("t", "b"),
		"SELECT `a`.`id` FROM `t` AS `a`, `t` AS `b`")

	fixture := &scanFixture{}
	db := fixtureDB(t, fixture)
	b := db.Select("value").From("t").Pagination(PageSize(3, 10))
	r := b.QueryRowContext(context.Background())
	_ = r.Close()

	if fixture.query != `SELECT "value" FROM "t" LIMIT 1 OFFSET 20` {
		t.Fatal(fixture.query)
	}

	checkSQL(t, b, `SELECT "value" FROM "t" LIMIT 10 OFFSET 20`)
	checkSQL(t, b.Pagination(PageSize(2, 5)).Offset(7), `SELECT "value" FROM "t" LIMIT 5 OFFSET 7`)
}
