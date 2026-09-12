// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx_test

import (
	"testing"

	"github.com/xgfone/go-sqlx"
	"github.com/xgfone/go-sqlx/dialect"
)

type legacyTenant int

func (v legacyTenant) BuildCondition(c *sqlx.BuildContext) string {
	return c.Quote("tenant") + "=" + c.Add(int(v))
}

type legacyIncrement int

func (v legacyIncrement) BuildUpdate(c *sqlx.BuildContext) string {
	return c.Quote("n") + "=" + c.Value(int(v))
}

var _ sqlx.LegacyCondition = legacyTenant(0)
var _ sqlx.LegacyUpdater = legacyIncrement(0)

func TestLegacyClauseAdapters(t *testing.T) {
	assertSQL(t,
		sqlx.Update().Table("t").Set(sqlx.AdaptUpdater(legacyIncrement(2))).
			Where(sqlx.AdaptCondition(legacyTenant(3))).SetDialect(dialect.Postgres),
		`UPDATE "t" SET "n"=$1 WHERE "tenant"=$2`,
		2, 3)
	if sqlx.AdaptCondition(nil) != nil || sqlx.AdaptUpdater(nil) != nil {
		t.Fatal("nil adapter")
	}

	c := sqlx.NewBuildContext(dialect.Postgres)
	got := sqlx.BuildCondition(c, sqlx.AdaptCondition(legacyTenant(4)))
	if got != `"tenant"=$1` {
		t.Fatal(got)
	}

	got = sqlx.BuildUpdate(c, sqlx.AdaptUpdater(legacyIncrement(5)))
	if got != `"n"=$2` {
		t.Fatal(got)
	}
}

func ExampleConditionWriterFunc() {
	predicate := sqlx.ConditionWriterFunc(func(w *sqlx.SQLWriter) (bool, error) {
		w.Raw("(")
		w.Path("score")
		w.Raw(">")
		w.Arg(10)
		w.Raw(")")
		return true, nil
	})
	_ = sqlx.Select("id").From("items").Where(predicate)
}
