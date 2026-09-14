// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"testing"

	"github.com/xgfone/go-sqlx/dialect"
)

func TestWindowGroupsDialectOrdering(t *testing.T) {
	frame := Window().Groups(UnboundedPreceding(), CurrentRow())
	for _, d := range []Dialect{dialect.SQLite, dialect.WithVersion(dialect.SQLite, 3, 39, 0)} {
		checkSQL(t, Select().SelectExpr(Sum("v").Over(frame)).From("t").SetDialect(d),
			`SELECT SUM("v") OVER (GROUPS BETWEEN UNBOUNDED PRECEDING AND CURRENT ROW) FROM "t"`)
		checkSQL(t, Select().SelectExpr(Sum("v").OverName("w")).From("t").Window("w", frame).SetDialect(d),
			`SELECT SUM("v") OVER "w" FROM "t" WINDOW "w" AS (GROUPS BETWEEN UNBOUNDED PRECEDING AND CURRENT ROW)`)
	}

	for _, d := range []Dialect{dialect.Postgres, dialect.WithVersion(dialect.Postgres, 14, 0, 0)} {
		checkBuildError(t, Select().SelectExpr(Sum("v").Over(frame)).SetDialect(d))
		checkBuildError(t, Select().SelectExpr(Sum("v").OverName("w")).Window("w", frame).SetDialect(d))
		checkSQL(t, Select().SelectExpr(Sum("v").Over(frame.BasedOn("w"))).From("t").
			Window("w", Window().OrderBy("v", Asc)).SetDialect(d),
			`SELECT SUM("v") OVER ("w" GROUPS BETWEEN UNBOUNDED PRECEDING AND CURRENT ROW) FROM "t" WINDOW "w" AS (ORDER BY "v" ASC)`)
	}

	grammar := dialect.SQLite.Grammar()
	grammar.WindowGroupsRequiresOrder = true
	checkBuildError(t, Select().SelectExpr(Sum("v").Over(frame)).
		SetDialect(dialect.WithGrammar(dialect.SQLite, grammar)))
	checkBuildError(t, Select().SelectExpr(Sum("v").Over(frame)).SetDialect(dialect.MySQL))
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
