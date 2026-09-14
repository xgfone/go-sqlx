// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"reflect"
	"testing"

	"github.com/xgfone/go-sqlx/dialect"
)

func TestUpdateJoinAndDeleteGrammar(t *testing.T) {
	update := Update().Table("t").Join("u", "", Eq("u.kind", "kind")).
		Set(Set("t.value", 7)).Where(Eq("t.id", 1))
	q, args := update.MustBuild()
	want := "UPDATE `t` INNER JOIN `u` ON (`u`.`kind` = ?) SET `t`.`value`=? WHERE (`t`.`id` = ?)"
	if q != want || !reflect.DeepEqual(args, []any{"kind", 7, 1}) {
		t.Fatalf("%q %#v", q, args)
	}

	mustPanic(t, func() { update.SetDB(&DB{Dialect: dialect.Postgres}).MustBuild() })
	from := Update().Table("t").From("u").Set(Set("value", 7)).SetDB(&DB{Dialect: dialect.Postgres})
	if q, _ := from.MustBuild(); q != `UPDATE "t" SET "value"=$1 FROM "u"` {
		t.Fatal(q)
	}

	mustPanic(t, func() { from.SetDB(&DB{Dialect: dialect.MySQL}).MustBuild() })
	deletion := Delete().FromAlias("t", "a").Join("u", "b", On("a.id", "b.id"))
	q, _ = deletion.MustBuild()
	if q != "DELETE `a` FROM `t` AS `a` INNER JOIN `u` AS `b` ON `a`.`id`=`b`.`id`" {
		t.Fatal(q)
	}

	mustPanic(t, func() { deletion.SetDB(&DB{Dialect: dialect.Postgres}).MustBuild() })
	mustPanic(t, func() { Insert().Into("t").Ignore().Values(1).SetDB(&DB{Dialect: dialect.Postgres}).MustBuild() })
	mustPanic(t, func() { Insert().Into("t").Values(1).Values(1, 2).MustBuild() })
}

func TestNestedBindingsAndJoinConditions(t *testing.T) {
	pg := &DB{Dialect: dialect.Postgres}
	sub := Select("id").From("u").Where(Eq("active", true))
	b := pg.Select().SelectExpr(Expr("COALESCE(?, ?)", Ident("a", "name"), "unknown")).
		FromSelect(sub, "a").
		JoinLeft("v", "b", Or(On("a.id", "b.id"), ConditionWriterFunc(func(w *SQLWriter) (bool, error) {
			w.Path("b.score")
			w.Raw(">")
			w.Arg(10)
			return true, nil
		}))).
		Where(Exists(Select().SelectExpr(Expr("1")).From("w").Where(Eq("kind", "x")))).
		Having(Expr("COUNT(*) > ?", 2).Condition())

	checkSQL(t, b,
		`SELECT COALESCE("a"."name", $1) FROM (SELECT "id" FROM "u" WHERE ("active" = $2)) AS "a" LEFT JOIN "v" AS "b" ON ("a"."id"="b"."id" OR "b"."score">$3) WHERE (EXISTS (SELECT 1 FROM "w" WHERE ("kind" = $4))) HAVING (COUNT(*) > $5)`,
		"unknown", true, 10, "x", 2)

	checkSQL(t, pg.Select("id").From("t").Where(InQuery("id", sub)),
		`SELECT "id" FROM "t" WHERE "id" IN (SELECT "id" FROM "u" WHERE ("active" = $1))`,
		true)

	checkSQL(t, pg.Update().Table("t").SetExpr("n", Expr("? + ?", Ident("n"), 3)).Where(Eq("id", 4)),
		`UPDATE "t" SET "n"="n" + $1 WHERE ("id" = $2)`,
		3, 4)
}

func TestReusableSourcesAndJoinKinds(t *testing.T) {
	rows := [][]any{{1, "a"}, {2, "b"}}
	cols := []string{"id", "name"}
	source := ValuesSource("v", cols, rows...)
	rows[0][0] = 99
	cols[0] = "changed"

	checkSQL(t,
		Select("v.id").FromSource(source).SetDialect(dialect.Postgres),
		`SELECT "v"."id" FROM (VALUES (CAST($1 AS BIGINT), CAST($2 AS TEXT)), (CAST($3 AS BIGINT), CAST($4 AS TEXT))) AS "v" ("id", "name")`,
		1, "a", 2, "b")
	checkSQL(t,
		Select("v.id").FromSource(source).SetDialect(dialect.WithVersion(dialect.MySQL, 8, 0, 19)),
		"SELECT `v`.`id` FROM (VALUES ROW(?, ?), ROW(?, ?)) AS `v` (`id`, `name`)",
		1, "a", 2, "b")
	checkSQL(t,
		Select("v.id").FromSource(source).SetDialect(dialect.SQLite),
		`SELECT "v"."id" FROM (SELECT ? AS "id", ? AS "name" UNION ALL SELECT ?, ?) AS "v"`,
		1, "a", 2, "b")
	checkSQL(t,
		Select("v.id").FromSource(source).SetDialect(dialect.MySQL),
		"SELECT `v`.`id` FROM (SELECT ? AS `id`, ? AS `name` UNION ALL SELECT ?, ?) AS `v`",
		1, "a", 2, "b")
	checkSQL(t,
		Select("v.id").FromSource(ValuesSource("v", []string{"id", "name"},
			[]any{1, Coalesce(Value(nil), Value("a"))}, []any{2, "b"}, []any{3, "c"})).SetDialect(dialect.SQLite),
		`SELECT "v"."id" FROM (SELECT column1 AS "id", column2 AS "name" FROM (VALUES (?, COALESCE(?, ?)), (?, ?), (?, ?))) AS "v"`,
		1, nil, "a", 2, "b", 3, "c")

	sub := Select("id").From("r").Where(Eq("v", 4))
	checkSQL(t,
		Select("a.id").FromAlias("a", "a").
			JoinLeftSelect(sub, "b", On("a.id", "b.id")).
			SetDialect(dialect.Postgres),
		`SELECT "a"."id" FROM "a" AS "a" LEFT JOIN (SELECT "id" FROM "r" WHERE ("v" = $1)) AS "b" ON "a"."id"="b"."id"`,
		4)
	checkSQL(t,
		Select("id").From("a").JoinFullUsing("b", "", "id").SetDialect(dialect.Postgres),
		`SELECT "id" FROM "a" FULL JOIN "b" USING ("id")`)
	checkSQL(t,
		Select("a.id").From("a").
			JoinSource(LeftJoin, QuerySource(sub, "b").Lateral(), Expr("TRUE").Condition()).
			SetDialect(dialect.Postgres),
		`SELECT "a"."id" FROM "a" LEFT JOIN LATERAL (SELECT "id" FROM "r" WHERE ("v" = $1)) AS "b" ON (TRUE)`,
		4)
	checkSQL(t,
		Update().Table("t").Set(Set("v", Ident("q", "v"))).
			FromSource(ValuesSource("q", []string{"v"}, []any{2})).
			SetDialect(dialect.Postgres),
		`UPDATE "t" SET "v"="q"."v" FROM (VALUES (CAST($1 AS BIGINT))) AS "q" ("v")`,
		2)

	checkBuildError(t, Select("*").FromSource(QuerySource(sub, "b").Lateral()).SetDialect(dialect.SQLite))
	checkBuildError(t, Select("*").FromSource(ValuesSource("v", []string{"a", "b"}, []any{1})))
	checkBuildError(t, Select("*").From("a").JoinSource(CrossJoinType, TableSource("b", ""), Eq("a.id", 1)))
	checkBuildError(t, Select("*").From("a").JoinSource("invalid", TableSource("b", ""), Eq("a.id", 1)))
}
