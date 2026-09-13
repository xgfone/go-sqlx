// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx_test

import (
	"testing"

	"github.com/xgfone/go-sqlx"
	"github.com/xgfone/go-sqlx/dialect"
)

type externalTenantCondition int

func (v externalTenantCondition) WriteCondition(w *sqlx.SQLWriter) (bool, error) {
	w.Path("tenant")
	w.Raw("=")
	w.Arg(int(v))
	return true, nil
}

type externalIncrementUpdater int

func (v externalIncrementUpdater) WriteUpdate(w *sqlx.SQLWriter) (bool, error) {
	w.Path("n")
	w.Raw("=")
	w.Value(int(v))
	return true, nil
}

var _ sqlx.Condition = externalTenantCondition(0)
var _ sqlx.Updater = externalIncrementUpdater(0)

func TestExternalClauseWriters(t *testing.T) {
	assertSQL(t,
		sqlx.Update().Table("t").Set(externalIncrementUpdater(2)).
			Where(externalTenantCondition(3)).SetDialect(dialect.Postgres),
		`UPDATE "t" SET "n"=$1 WHERE "tenant"=$2`,
		2, 3)
	c := sqlx.NewBuildContext(dialect.Postgres)
	got := sqlx.BuildCondition(c, externalTenantCondition(4))
	if got != `"tenant"=$1` {
		t.Fatal(got)
	}

	got = sqlx.BuildUpdate(c, externalIncrementUpdater(5))
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
