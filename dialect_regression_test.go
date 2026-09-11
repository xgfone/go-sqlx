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
