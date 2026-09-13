// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"testing"

	"github.com/xgfone/go-sqlx/dialect"
)

func TestQueryLevelAggregateAndWindowValidation(t *testing.T) {
	custom := ConditionWriterFunc(func(w *SQLWriter) (bool, error) {
		w.Raw(w.ctx.Value(Sum("v")))
		w.Raw(" > 0")
		return true, nil
	})
	for _, e := range []Expression{
		Coalesce(Sum("v"), 0), Cast(Max("v"), "INTEGER"), Func("abs", Min("v")),
		NullIf(CountExpr(Ident("v")), 0), Expr("? + ?", Sum("v"), 1),
		Case().When(Gt(Sum("v"), 0), 1).Else(0).End(),
		CaseValue(Count("*")).WhenValue(0, 1).Else(2).End(),
		Coalesce(Sum("v").Filter(Gt("v", 0)), 0),
		Coalesce(RowNumber().Over(Window()), 0),
		Cast(Sum("v").Over(Window()), "INTEGER"),
		Case().When(custom, 1).Else(0).End(), Tuple(Sum("v"), Value(1)),
	} {
		for _, d := range []Dialect{dialect.Postgres, dialect.SQLite} {
			checkBuildError(t, Insert().Into("t").Columns("v").Values(1).ReturningExpr(e, "r").SetDialect(d))
			checkBuildError(t, Update().Table("t").Set(Set("v", 1)).ReturningExpr(e, "r").SetDialect(d))
			checkBuildError(t, Delete().From("t").ReturningExpr(e, "r").SetDialect(d))
		}
		checkBuildError(t, Select().SelectExpr(e).From("t").ForUpdate().SetDialect(dialect.Postgres))
		checkBuildError(t, Select("v").From("t").OrderByExpr(e, Asc).ForShare().SetDialect(dialect.Postgres))
	}

	// Scalar subqueries are separate query levels and remain valid.
	e := Coalesce(Subquery(Select().SelectExpr(Sum("v")).From("other")), 0)
	checkSQL(t, Update().Table("t").Set(Set("v", 1)).ReturningExpr(e, "total").SetDialect(dialect.Postgres),
		`UPDATE "t" SET "v"=$1 RETURNING COALESCE((SELECT SUM("v") FROM "other"), $2) AS "total"`, 1, 0)
	checkSQL(t, Select().SelectExpr(e).From("t").ForUpdate().SetDialect(dialect.Postgres),
		`SELECT COALESCE((SELECT SUM("v") FROM "other"), $1) FROM "t" FOR UPDATE`, 0)
	// Leaving a subquery must restore the outer restriction.
	checkBuildError(t, Update().Table("t").Set(Set("v", 1)).
		ReturningExpr(Coalesce(Subquery(Select().SelectExpr(Value(1))), Sum("v")), "r").SetDialect(dialect.Postgres))
}

func TestSQLiteReturningQualification(t *testing.T) {
	for _, col := range []string{"t.*", "u.v", "other.v", "main.t.v"} {
		for _, d := range []Dialect{dialect.SQLite, dialect.WithVersion(dialect.SQLite, 3, 39, 0)} {
			checkBuildError(t, Insert().IntoAlias("t", "u").Columns("v").Values(1).Returning(col).SetDialect(d))
			checkBuildError(t, Update().TableAlias("t", "u").Set(Set("v", 1)).Returning(col).SetDialect(d))
			checkBuildError(t, Delete().FromAlias("t", "u").Returning(col).SetDialect(d))
		}
	}

	for _, e := range []Expression{
		Coalesce(Ident("u", "v"), 0), Cast(Ident("u", "v"), "INTEGER"),
		Case().When(Eq("u.v", 1), 2).Else(3).End(),
		Case().When(On("u.v", "t.v"), 2).Else(3).End(),
		Expr("? + ?", Ident("u", "v"), 1),
		Case().When(ConditionWriterFunc(func(w *SQLWriter) (bool, error) {
			w.Ident("u", "v")
			w.Raw(" = 1")
			return true, nil
		}), 2).Else(3).End(),
	} {
		checkBuildError(t, Update().TableAlias("t", "u").Set(Set("v", 1)).ReturningExpr(e, "r").SetDialect(dialect.SQLite))
	}

	checkSQL(t, Update().TableAlias("main.t", "u").Set(Set("v", 1)).Returning("*", "t.v").
		ReturningExpr(Coalesce(Ident("t", "v"), 0), "r").SetDialect(dialect.SQLite),
		`UPDATE "main"."t" AS "u" SET "v"=? RETURNING *, "t"."v", COALESCE("t"."v", ?) AS "r"`, 1, 0)
	checkSQL(t, Update().Table("T").Set(Set("v", 1)).Returning("t.v").SetDialect(dialect.SQLite),
		`UPDATE "T" SET "v"=? RETURNING "t"."v"`, 1)
	checkSQL(t, Update().TableAlias("t", "u").Set(Set("v", 1)).Returning("u.*", "u.v").SetDialect(dialect.Postgres),
		`UPDATE "t" AS "u" SET "v"=$1 RETURNING "u".*, "u"."v"`, 1)
	checkSQL(t, Update().Table("t").Set(Set("v", 1)).ReturningExpr(
		Subquery(Select("o.v").FromAlias("other", "o").Limit(1)), "r").SetDialect(dialect.SQLite),
		`UPDATE "t" SET "v"=? RETURNING (SELECT "o"."v" FROM "other" AS "o" LIMIT 1) AS "r"`, 1)
}
