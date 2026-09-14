// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"testing"

	"github.com/xgfone/go-sqlx/dialect"
)

func TestExpressionPredicateGrouping(t *testing.T) {
	checkSQL(t, Select("id").From("t").Where(Expr("a=? OR b=?", 1, 2).Condition(), Eq("tenant", 3)),
		"SELECT `id` FROM `t` WHERE ((a=? OR b=?) AND (`tenant` = ?))", 1, 2, 3)
	checkSQL(t, Delete().From("t").Where(Eq("deleted_at", nil)), "DELETE FROM `t` WHERE (`deleted_at` IS NULL)")
	checkBuildError(t, Update().Table("t").Set(Set("v", 1)).Where(ConditionWriterFunc(func(*SQLWriter) (bool, error) { panic("invalid comparison") })))
}

func TestStreamingCustomConditionsCalledOnce(t *testing.T) {
	calls := 0
	custom := ConditionWriterFunc(func(w *SQLWriter) (bool, error) {
		calls++
		w.Arg(7)
		w.Raw("=7")
		return true, nil
	})
	checkSQL(t, Select("id").Where(Or(nil, And(), custom), Eq("v", 8)).SetDialect(dialect.Postgres),
		`SELECT "id" WHERE ($1=7 AND ("v" = $2))`, 7, 8)
	if calls != 1 {
		t.Fatalf("custom condition evaluated %d times", calls)
	}
}
