// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"reflect"
	"strings"
	"testing"

	"github.com/xgfone/go-sqlx/dialect"
)

func TestNativeDefaultContexts(t *testing.T) {
	for _, d := range []Dialect{dialect.Postgres, dialect.MySQL} {
		q, args, err := Update().Table("t").Set(Set("v", Default())).SetDialect(d).Build()
		if err != nil || !strings.HasSuffix(q, "=DEFAULT") || len(args) != 0 {
			t.Fatalf("%s %v %v", q, args, err)
		}
	}

	for _, b := range []SQLBuilder{
		Update().Table("t").Set(Set("v", Default())).SetDialect(dialect.SQLite),
		Insert().Into("t").Values(1).OnConflict(ConflictColumns().DoUpdate(Set("v", Default()))).SetDialect(dialect.SQLite),
		Select().SelectExpr(Default()),
		Insert().Into("t").Values(Expr("COALESCE(?, ?)", Default(), 1)),
	} {
		checkBuildError(t, b)
	}

	checkSQL(t,
		Insert().Into("t").DefaultValues().OnDuplicateKeyUpdate(Set("v", 1)),
		"INSERT INTO `t` () VALUES () ON DUPLICATE KEY UPDATE `v`=?",
		1,
	)
}

func TestDistinctOnFetchAndMutationLimits(t *testing.T) {
	q := Select("team", "v").From("t").DistinctOn("team").OrderByAsc("team").OrderByDesc("v").SetDialect(dialect.Postgres)
	checkSQL(t, q, `SELECT DISTINCT ON ("team") "team", "v" FROM "t" ORDER BY "team" ASC, "v" DESC`)
	checkBuildError(t, q.Clone().Distinct())
	checkBuildError(t, q.Clone().SetDialect(dialect.MySQL))
	checkSQL(t,
		Select("id").From("t").OrderByAsc("id").Offset(2).FetchWithTies(3).SetDialect(dialect.Postgres),
		`SELECT "id" FROM "t" ORDER BY "id" ASC OFFSET 2 ROWS FETCH FIRST 3 ROWS WITH TIES`)

	checkBuildError(t, Select("id").FetchWithTies(2).SetDialect(dialect.Postgres))
	checkBuildError(t, Select("id").OrderByAsc("id").FetchWithTies(2))
	checkSQL(t,
		Select("id").From("t").ForKeyShare("t").NoWait().SetDialect(dialect.Postgres),
		`SELECT "id" FROM "t" FOR KEY SHARE OF "t" NOWAIT`)

	checkBuildError(t, Select("id").From("t").ForNoKeyUpdate())
	checkSQL(t,
		Update().Table("t").Set(Set("v", 1)).OrderByAsc("id").Limit(2),
		"UPDATE `t` SET `v`=? ORDER BY `id` ASC LIMIT 2", 1)
	checkSQL(t,
		Delete().From("t").OrderByDesc("id").Limit(0),
		"DELETE FROM `t` ORDER BY `id` DESC LIMIT 0")

	checkBuildError(t, Update().Table("t").Join("u", "", On("t.id", "u.id")).Set(Set("v", 1)).Limit(1))
	checkBuildError(t, Delete().From("t").Limit(1).SetDialect(dialect.SQLite))
	checkBuildError(t, Delete().From("t").Limit(1).SetDialect(dialect.Postgres))
	optional := dialect.WithFeatures(dialect.SQLite, []dialect.Feature{dialect.DeleteOrderLimit}, nil)
	checkSQL(t,
		Delete().From("t").Returning("id").OrderByAsc("id").Limit(1).SetDialect(optional),
		`DELETE FROM "t" RETURNING "id" ORDER BY "id" ASC LIMIT 1`)
}

func TestNewClauseCloneIsolationAndClears(t *testing.T) {
	e := Ident("id")
	terms := SortColumns{{Expr: &e, Order: Asc, Nulls: NullsLast}}
	b := Select("id").From("t").Sort(terms).Window("w", Window()).SetDialect(dialect.Postgres)

	e = Ident("changed")
	terms[0].Nulls = NullsFirst
	clone := b.Clone().ClearWindows().ClearOrderBy().OrderByDesc("id")
	checkSQL(t, b, `SELECT "id" FROM "t" WINDOW "w" AS () ORDER BY "id" ASC NULLS LAST`)
	checkSQL(t, clone, `SELECT "id" FROM "t" ORDER BY "id" DESC`)

	update := Update().Table("t").Set(Set("v", 1)).OrderByAsc("id").Limit(2)
	checkSQL(t, update.Clone().ClearOrderBy().ClearLimit(), "UPDATE `t` SET `v`=?", 1)
	checkSQL(t, update, "UPDATE `t` SET `v`=? ORDER BY `id` ASC LIMIT 2", 1)

	insert := Insert().Into("t").Values(1).OnConflict(ConflictColumns("id").DoNothing()).SetDialect(dialect.Postgres)
	checkSQL(t, insert.Clone().ClearConflict(), `INSERT INTO "t" VALUES ($1)`, 1)
	checkSQL(t, insert, `INSERT INTO "t" VALUES ($1) ON CONFLICT ("id") DO NOTHING`, 1)
	if !reflect.DeepEqual(Window().orders, []SortColumn(nil)) {
		t.Fatal("zero window mutated")
	}
}

func TestDialectExtensionsKeepParentBindingOrder(t *testing.T) {
	cte := NewCTE("candidate", Select().SelectExpr(Value(5)), "id")
	q := Delete().From("t").WithCTE(cte).
		UsingSelect(Select().SelectExpr(Value(6)), "q").Where(Eq("id", 7)).
		Returning("id").SetDialect(dialect.Postgres)
	checkSQL(t, q,
		`WITH "candidate" ("id") AS (SELECT $1) DELETE FROM "t" USING (SELECT $2) AS "q" WHERE ("id" = $3) RETURNING "id"`,
		5, 6, 7)
	checkSQL(t,
		Select().SelectExpr(Sum("v")).From("t").
			GroupByExpr(Cube(Ident("a"), Ident("b"))).
			SetDialect(dialect.Postgres),
		`SELECT SUM("v") FROM "t" GROUP BY CUBE ("a", "b")`)
	checkSQL(t,
		Select().SelectExpr(Value(1)).
			IntersectAll(Select().SelectExpr(Value(2))).
			SetDialect(dialect.Postgres),
		`SELECT $1 INTERSECT ALL SELECT $2`,
		1, 2)
	checkSQL(t,
		Select("id").From("t").ForNoKeyUpdate("t").SkipLocked().SetDialect(dialect.Postgres),
		`SELECT "id" FROM "t" FOR NO KEY UPDATE OF "t" SKIP LOCKED`)
	checkSQL(t,
		Update().Table("t").Set(SetRow([]string{"a", "b"}, 1, Default())).
			SetDialect(dialect.Postgres),
		`UPDATE "t" SET ("a", "b")=($1, DEFAULT)`,
		1)
}
