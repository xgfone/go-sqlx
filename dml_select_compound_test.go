// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"testing"

	"github.com/xgfone/go-sqlx/dialect"
)

func TestMixedSetOperationsKeepCTEScope(t *testing.T) {
	q := Select("v").With("q", Select().SelectExpr(Value(1)), "v").From("q").
		Union(Select("v").From("q")).Intersect(Select("v").From("q")).
		Except(Select("v").From("q")).OrderByAsc("v")
	checkSQL(t, q.Clone().SetDialect(dialect.Postgres),
		`WITH "q" ("v") AS (SELECT $1) ((SELECT "v" FROM "q" UNION SELECT "v" FROM "q") INTERSECT SELECT "v" FROM "q") EXCEPT SELECT "v" FROM "q" ORDER BY "v" ASC`, 1)
	checkSQL(t, q.Clone().SetDialect(dialect.SQLite),
		`WITH "q" ("v") AS (SELECT ?) SELECT * FROM (SELECT * FROM (SELECT "v" FROM "q" UNION SELECT "v" FROM "q") AS "_sqlx_set" INTERSECT SELECT "v" FROM "q") AS "_sqlx_set" EXCEPT SELECT "v" FROM "q" ORDER BY "v" ASC`, 1)
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
