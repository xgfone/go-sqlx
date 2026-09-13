// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/xgfone/go-sqlx/dialect"
)

func TestDialectLexicalBoundaries(t *testing.T) {
	for _, d := range []Dialect{dialect.Postgres, dialect.SQLite} {
		want := `SELECT '\', ?`
		if d == dialect.Postgres {
			want = `SELECT '\', $1`
		}
		checkSQL(t, Select().SelectExpr(Expr(`'\', ?`, 1)).SetDialect(d), want, 1)
	}

	checkSQL(t, Select().SelectExpr(Expr(`?--?`, 3, 1)), `SELECT ?--?`, 3, 1)
	checkSQL(t, Select().SelectExpr(Expr("? # ignored ?\n + ?", 3, 1)), "SELECT ? # ignored ?\n + ?", 3, 1)
	checkSQL(t, Select().SelectExpr(Expr(`E'it\'s ?' || ?`, "x")).SetDialect(dialect.Postgres), `SELECT E'it\'s ?' || $1`, "x")
	checkSQL(t, Select().SelectExpr(Expr(`/* outer /* inner ? */ ? */ ?`, 2)).SetDialect(dialect.Postgres), `SELECT /* outer /* inner ? */ ? */ $1`, 2)
	checkSQL(t, Select().SelectExpr(Expr(`'it\'s ?' || ?`, "x")), `SELECT 'it\'s ?' || ?`, "x")

	rules := dialect.MySQL.LexicalRules()
	rules.BackslashStrings = false
	checkSQL(t, Select().SelectExpr(Expr(`'\', ?`, 1)).SetDialect(dialect.WithLexicalRules(dialect.MySQL, rules)), `SELECT '\', ?`, 1)
	checkBuildError(t, Select().SelectExpr(Expr(`? /* unclosed`, 1)))
	checkBuildError(t, Select().SelectExpr(Expr(`? /* outer /* inner */ ? */`, 1)).SetDialect(dialect.SQLite))
}

func TestNativeDefaultContexts(t *testing.T) {
	for _, d := range []Dialect{dialect.Postgres, dialect.MySQL} {
		q, args, err := Update().Table("t").Set(Set("v", Default())).SetDialect(d).Build()
		if err != nil || !strings.HasSuffix(q, "=DEFAULT") || len(args) != 0 {
			t.Fatalf("%s %v %v", q, args, err)
		}
	}

	for _, b := range []SQLBuilder{
		Update().Table("t").Set(Set("v", Default())).SetDialect(dialect.SQLite),
		Insert().Into("t").Values(1).OnConflictDoUpdate(nil, Set("v", Default())).SetDialect(dialect.SQLite),
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

func TestConditionalUpsertsAndAliases(t *testing.T) {
	pg := dialect.Postgres
	q := Insert().IntoAlias("users", "u").Columns("email", "v").Values("a", 2).
		OnConflict(ConflictExpressions(Func("lower", Ident("email"))).
			Where(IsNull("deleted_at")).DoUpdate(Set("v", Excluded("v"))).
			Where(Lt("u.v", Excluded("v")))).Returning("v").SetDialect(pg)

	checkSQL(t,
		q,
		`INSERT INTO "users" AS "u" ("email", "v") VALUES ($1, $2) ON CONFLICT ((lower("email"))) WHERE ("deleted_at" IS NULL) DO UPDATE SET "v"="excluded"."v" WHERE ("u"."v" < "excluded"."v") RETURNING "v"`,
		"a", 2)
	checkSQL(t,
		Insert().Into("t").Values(1).OnConflict(ConflictConstraint("t_pkey").DoNothing()).SetDialect(pg),
		`INSERT INTO "t" VALUES ($1) ON CONFLICT ON CONSTRAINT "t_pkey" DO NOTHING`,
		1)
	checkSQL(t,
		Insert().Into("t").Values(1).OnConflictDoUpdate(nil, Set("v", 2)).SetDialect(dialect.SQLite),
		`INSERT INTO "t" VALUES (?) ON CONFLICT DO UPDATE SET "v"=?`,
		1, 2)

	my := dialect.WithVersion(dialect.MySQL, 8, 0, 19)
	checkSQL(t,
		Insert().Into("t").Columns("id", "v").Values(1, 2).RowsAlias("new").OnDuplicateKeyUpdate(Set("v", Inserted("v"))).SetDialect(my),
		"INSERT INTO `t` (`id`, `v`) VALUES (?, ?) AS `new` ON DUPLICATE KEY UPDATE `v`=`new`.`v`",
		1, 2)
	checkSQL(t,
		Insert().Into("t").Columns("id", "v").Values(1, 2).RowsAlias("new", "a", "b").OnDuplicateKeyUpdate(Set("v", Inserted("b"))).SetDialect(my),
		"INSERT INTO `t` (`id`, `v`) VALUES (?, ?) AS `new` (`a`, `b`) ON DUPLICATE KEY UPDATE `v`=`new`.`b`",
		1, 2)

	for _, b := range []SQLBuilder{
		q.Clone().SetDialect(dialect.MySQL),
		Insert().Into("t").Values(1).OnConflictDoUpdate(nil, Set("v", 2)).SetDialect(pg),
		Insert().Into("t").Values(1).OnConflict(ConflictColumns().DoNothing(), ConflictColumns("id").DoNothing()).SetDialect(dialect.SQLite),
		Insert().Into("t").Values(1).OnConflict(ConflictColumns("id").DoNothing(), ConflictColumns().DoNothing()).SetDialect(pg),
		Insert().Into("t").Values(1).OnConflict(ConflictColumns("id").DoNothing().Where(Eq("id", 1))).SetDialect(pg),
		Insert().Into("t").Values(1).RowsAlias("new"),
		Insert().Into("t").Values(1).RowsAlias("t").SetDialect(my),
		Insert().Into("t").Values(1).RowsAlias("new", "a", "b").SetDialect(my),
		Select().SelectExpr(Excluded("v")).SetDialect(pg),
		Select().SelectExpr(Inserted("v")).SetDialect(my),
	} {
		checkBuildError(t, b)
	}
}

func TestInsertSelectConflictDialectRules(t *testing.T) {
	source := Select("id").From("src")
	checkSQL(t,
		Insert().Into("t").FromSelect(source).OnConflictDoNothing().SetDialect(dialect.Postgres),
		`INSERT INTO "t" SELECT "id" FROM "src" ON CONFLICT DO NOTHING`)
	checkSQL(t,
		Insert().Into("t").FromSelect(source).OnDuplicateKeyUpdate(Set("id", 1)),
		"INSERT INTO `t` SELECT `id` FROM `src` ON DUPLICATE KEY UPDATE `id`=?",
		1)

	for _, source := range []*SelectBuilder{
		source,
		source.Clone().Where(Eq("id", 1)).UnionAll(Select("id").From("src")),
	} {
		q, _, e := Insert().Into("t").FromSelect(source).OnConflictDoNothing().SetDialect(dialect.SQLite).Build()
		if e != nil || !strings.Contains(q, `) AS "_sqlx_insert" WHERE (TRUE) ON CONFLICT`) {
			t.Fatalf("%s %v", q, e)
		}
	}

	if len(source.wheres) > 0 {
		t.Fatal("insert changed source")
	}
}

func TestSetOperationsPrecedenceAndOperandPagination(t *testing.T) {
	a := Select().SelectExpr(Value(1))
	b := Select().SelectExpr(Value(2))
	c := Select().SelectExpr(Value(3))

	checkSQL(t,
		a.Clone().Union(b).Intersect(c).SetDialect(dialect.Postgres),
		`(SELECT $1 UNION SELECT $2) INTERSECT SELECT $3`,
		1, 2, 3)
	checkSQL(t,
		a.Clone().Union(b.Clone().Except(c)).SetDialect(dialect.Postgres),
		`SELECT $1 UNION (SELECT $2 EXCEPT SELECT $3)`,
		1, 2, 3)
	checkSQL(t,
		a.Clone().UnionAll(b.Clone().OrderByExpr(Expr("1"), Asc).Limit(1)).SetDialect(dialect.Postgres),
		`SELECT $1 UNION ALL (SELECT $2 ORDER BY 1 ASC LIMIT 1)`,
		1, 2)
	checkSQL(t,
		a.Clone().ExceptAll(b).SetDialect(dialect.WithVersion(dialect.MySQL, 8, 0, 31)),
		`SELECT ? EXCEPT ALL SELECT ?`,
		1, 2)

	checkBuildError(t, a.Clone().Intersect(b))
	checkBuildError(t, a.Clone().IntersectAll(b).SetDialect(dialect.SQLite))
	checkBuildError(t, a.Clone().Union(Select("id").From("t").ForUpdate()).SetDialect(dialect.Postgres))
	checkSQL(t, a.Clone().Union(b).UnionAll(c).Intersect(b).IntersectAll(c).Except(b).ExceptAll(c).
		ClearSetOperations().SetDialect(dialect.SQLite), `SELECT ?`, 1)
}

func TestWindowAndGroupingRendering(t *testing.T) {
	w := Window().PartitionBy("team").Sort(SortColumn{Column: "score", Order: Desc, Nulls: NullsLast}).
		Rows(UnboundedPreceding(), CurrentRow())
	checkSQL(t,
		Select().SelectExpr(SumExpr(Expr("? * ?", Ident("v"), 2)).
			Filter(Gt("v", 0)).Over(w)).From("t").SetDialect(dialect.Postgres),
		`SELECT SUM("v" * $1) FILTER (WHERE ("v" > $2)) OVER (PARTITION BY "team" ORDER BY "score" DESC NULLS LAST ROWS BETWEEN UNBOUNDED PRECEDING AND CURRENT ROW) FROM "t"`,
		2, 0)
	checkSQL(t,
		Select().SelectExpr(RowNumber().OverName("w")).From("t").
			Window("w", Window().OrderBy("id", Asc)).SetDialect(dialect.SQLite),
		`SELECT ROW_NUMBER() OVER "w" FROM "t" WINDOW "w" AS (ORDER BY "id" ASC)`)
	checkSQL(t,
		Select("team").SelectExpr(Sum("v")).From("t").GroupByRollup("team").SetDialect(dialect.Postgres),
		`SELECT "team", SUM("v") FROM "t" GROUP BY ROLLUP ("team")`)
	checkSQL(t,
		Select("team").SelectExpr(Sum("v")).From("t").GroupByRollup("team"),
		"SELECT `team`, SUM(`v`) FROM `t` GROUP BY `team` WITH ROLLUP")
	checkSQL(t,
		Select().SelectExpr(Count("*")).From("t").
			GroupByExpr(GroupingSets(GroupingSet(Ident("team")), GroupingSet())).
			SetDialect(dialect.Postgres),
		`SELECT COUNT(*) FROM "t" GROUP BY GROUPING SETS (("team"), ())`)

	for _, b := range []SQLBuilder{
		Select().SelectExpr(Sum("v").Filter(Eq("id", 1))),
		Select().SelectExpr(RowNumber().Over(Window().Rows(UnboundedFollowing(), CurrentRow()))),
		Select().SelectExpr(RowNumber().Over(Window().Rows(Preceding(-1), CurrentRow()))),
		Select().SelectExpr(RowNumber().Over(Window().Range(Preceding(1), CurrentRow()))),
		Select().SelectExpr(RowNumber().Over(Window().OrderBy("id", Asc).Groups(CurrentRow(), CurrentRow()))),
		Select().SelectExpr(RowNumber().Over(Window().Rows(CurrentRow(), CurrentRow()).Exclude(ExcludeTies))),
		Select().SelectExpr(RowNumber().Over(Window().Exclude(ExcludeTies))).SetDialect(dialect.Postgres),
		Select("id").Sort(SortColumn{Column: "id", Nulls: NullsLast}),
		Select("id").Window("w", Window()).Window("w", Window()).SetDialect(dialect.Postgres),
		Select("id").Window("w", Window().BasedOn("missing")).SetDialect(dialect.Postgres),
		Select("id").GroupByRollup("id").SetDialect(dialect.SQLite),
	} {
		checkBuildError(t, b)
	}
}

func TestCTEPlacementAndDMLScope(t *testing.T) {
	cte := NewCTE("q", Select().SelectExpr(Value(7)), "id")
	checkSQL(t,
		Insert().Into("t").Columns("id").WithCTE(cte).FromSelect(Select("id").From("q")),
		"INSERT INTO `t` (`id`) WITH `q` (`id`) AS (SELECT ?) SELECT `id` FROM `q`",
		7)
	checkSQL(t,
		Insert().Into("t").Columns("id").WithCTE(cte).FromSelect(Select("id").
			From("q")).SetDialect(dialect.Postgres),
		`WITH "q" ("id") AS (SELECT $1) INSERT INTO "t" ("id") SELECT "id" FROM "q"`,
		7)

	deleted := Delete().From("old").Where(Eq("id", 9)).Returning("id")
	q := Select("id").From("deleted").WithCTE(NewCTE("deleted", deleted)).SetDialect(dialect.Postgres)
	checkSQL(t, q, `WITH "deleted" AS (DELETE FROM "old" WHERE ("id" = $1) RETURNING "id") SELECT "id" FROM "deleted"`, 9)

	deleted.ClearWhere()
	checkSQL(t, q, `WITH "deleted" AS (DELETE FROM "old" WHERE ("id" = $1) RETURNING "id") SELECT "id" FROM "deleted"`, 9)
	checkBuildError(t, q.Clone().SetDialect(dialect.SQLite))
	checkBuildError(t, Select("*").FromSelect(q, "nested").SetDialect(dialect.Postgres))
	checkBuildError(t, Insert().Into("t").WithCTE(cte).Values(1))
	checkBuildError(t, Select("*").WithCTE(cte.Materialized()))
	checkSQL(t,
		Select("id").From("q").WithCTE(cte.NotMaterialized()).SetDialect(dialect.SQLite),
		`WITH "q" ("id") AS NOT MATERIALIZED (SELECT ?) SELECT "id" FROM "q"`,
		7)
}

func TestReusableSourcesAndJoinKinds(t *testing.T) {
	rows := [][]any{{1, "a"}, {2, "b"}}
	cols := []string{"id", "name"}
	source := ValuesSource("v", cols, rows...)
	rows[0][0] = 99
	cols[0] = "changed"

	checkSQL(t,
		Select("v.id").FromSource(source).SetDialect(dialect.Postgres),
		`SELECT "v"."id" FROM (VALUES ($1, $2), ($3, $4)) AS "v" ("id", "name")`,
		1, "a", 2, "b")
	checkSQL(t,
		Select("v.id").FromSource(source).SetDialect(dialect.WithVersion(dialect.MySQL, 8, 0, 19)),
		"SELECT `v`.`id` FROM (VALUES ROW(?, ?), ROW(?, ?)) AS `v` (`id`, `name`)",
		1, "a", 2, "b")

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
		`UPDATE "t" SET "v"="q"."v" FROM (VALUES ($1)) AS "q" ("v")`,
		2)

	checkBuildError(t, Select("*").FromSource(QuerySource(sub, "b").Lateral()).SetDialect(dialect.SQLite))
	checkBuildError(t, Select("*").FromSource(ValuesSource("v", []string{"a", "b"}, []any{1})))
	checkBuildError(t, Select("*").From("a").JoinSource(CrossJoinType, TableSource("b", ""), Eq("a.id", 1)))
	checkBuildError(t, Select("*").From("a").JoinSource("invalid", TableSource("b", ""), Eq("a.id", 1)))
}

func TestDistinctOnFetchAndMutationLimits(t *testing.T) {
	q := Select("team", "v").From("t").DistinctOn("team").OrderByAsc("team").OrderByDesc("v").SetDialect(dialect.Postgres)
	checkSQL(t, q, `SELECT DISTINCT ON ("team") "team", "v" FROM "t" ORDER BY "team" ASC, "v" DESC`)
	checkBuildError(t, q.Clone().ClearOrderBy().OrderByAsc("v"))
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
	checkBuildError(t,
		Select("id").DistinctOnExpr(Coalesce(Ident("name"), "a")).
			OrderByExpr(Coalesce(Ident("name"), "b"), Asc).
			SetDialect(dialect.Postgres))
	checkSQL(t,
		Select().SelectExprAlias(e, "key").From("t").DistinctOnExpr(e).OrderByAsc("key").SetDialect(dialect.Postgres),
		`SELECT DISTINCT ON (COALESCE("name", $1)) COALESCE("name", $2) AS "key" FROM "t" ORDER BY COALESCE("name", $1) ASC`,
		"fallback", "fallback")
	checkSQL(t,
		Select("id").From("t").DistinctOn("id", "id").OrderByAsc("id").OrderByAsc("v").SetDialect(dialect.Postgres),
		`SELECT DISTINCT ON ("id", "id") "id" FROM "t" ORDER BY "id" ASC, "v" ASC`)
	checkSQL(t,
		Select("id").From("t").DistinctOn("id").OrderByExpr(Expr("1"), Asc).SetDialect(dialect.Postgres),
		`SELECT DISTINCT ON ("id") "id" FROM "t" ORDER BY "id" ASC`)
}

func TestWindowScopesAndInvalidCombinations(t *testing.T) {
	for _, b := range []SQLBuilder{
		Select().SelectExpr(RowNumber()).SetDialect(dialect.Postgres),
		Select().SelectExpr(RowNumber().OverName("missing")).SetDialect(dialect.Postgres),
		Select().SelectExpr(Sum("v").Filter(Eq("id", 1)).Filter(Eq("id", 2))).
			SetDialect(dialect.Postgres),
		Select().SelectExpr(CountDistinct("id").Over(Window())).SetDialect(dialect.Postgres),
		Select().SelectExpr(CountDistinctExpr(Ident("id")).
			Filter(Eq("id", 1)).Over(Window())).SetDialect(dialect.SQLite),
		Select().SelectExpr(RowNumber().Over(Window().BasedOn("w").Range(Preceding(1), CurrentRow()))).
			Window("w", Window()).SetDialect(dialect.Postgres),
		Select().SelectExpr(RowNumber().OverName("w")).Window("w", Window().BasedOn("w")).
			SetDialect(dialect.Postgres),
		Select().SelectExpr(RowNumber().OverName("c")).Window("a", Window().OrderBy("id", Asc)).
			Window("b", Window().BasedOn("a")).
			Window("c", Window().BasedOn("b").OrderBy("v", Asc)).
			SetDialect(dialect.Postgres),
		Select().SelectExpr(Subquery(Select().SelectExpr(RowNumber().OverName("outer")))).
			Window("outer", Window()).SetDialect(dialect.Postgres),
		Update().Table("t").Set(Set("v", 1)).ReturningExpr(Sum("v"), "total").
			SetDialect(dialect.Postgres),
		Insert().Into("t").Values(Expr("DEFAULT", 1)),
		Select("id").Where(In(Tuple(Ident("a"), Ident("b")), Tuple(1, 2, 3))),
		Select("id").Where(Gt(Tuple(Ident("a"), Ident("b")), Tuple(1, 2, 3))),
		Select().SelectExpr(Case().End()),
		Select().SelectExpr(CaseValue(1).When(Eq("id", 1), 2).End()),
		Select().SelectExpr(Case().WhenValue(1, 2).End()),
		Select("id").Where(Like("name", "x", "ab")),
		Select().SelectExpr(Coalesce(1)),
		Insert().Into("t").Values(1).OnConflict(ConflictConstraint("").DoNothing()).
			SetDialect(dialect.Postgres),
	} {
		checkBuildError(t, b)
	}

	checkSQL(t,
		Select().SelectExpr(Subquery(Select().SelectExpr(RowNumber().OverName("w")).
			Window("w", Window())), RowNumber().OverName("w")).Window("w", Window()).
			SetDialect(dialect.Postgres),
		`SELECT (SELECT ROW_NUMBER() OVER "w" WINDOW "w" AS ()), ROW_NUMBER() OVER "w" WINDOW "w" AS ()`)
}

func TestFetchWithTiesQueryRowUsesOrdinaryLimit(t *testing.T) {
	f := &scanFixture{}
	db := fixtureDB(t, f)
	b := db.Select("value").From("t").OrderByAsc("value").FetchWithTies(1).SetDialect(dialect.Postgres)
	r := b.QueryRowContext(context.Background())
	_ = r.Close()

	if strings.Contains(f.query, "WITH TIES") || !strings.HasSuffix(f.query, " LIMIT 1") {
		t.Fatal(f.query)
	}
	if !b.withTies {
		t.Fatal("QueryRow mutated the builder")
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
