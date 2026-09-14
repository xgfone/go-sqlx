// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"context"
	"reflect"
	"testing"

	"github.com/xgfone/go-sqlx/dialect"
)

func TestBuildConvertsFailure(t *testing.T) {
	failure := &struct{}{}
	condition := ConditionWriterFunc(func(w *SQLWriter) (bool, error) {
		w.Arg(7)
		panic(failure)
	})
	for _, builder := range []SQLBuilder{
		Select("*").From("t").Where(condition),
		Update().Table("t").Set(Set("value", 1)).Where(condition),
		Delete().From("t").Where(condition),
	} {
		if q, a, e := builder.Build(); e == nil || q != "" || a != nil {
			t.Fatalf("failed build: %q %v %v", q, a, e)
		}
	}

	q, args := Select("id").From("t").Where(Eq("id", 9)).MustBuild()
	if q != "SELECT `id` FROM `t` WHERE (`id` = ?)" || !reflect.DeepEqual(args, []any{9}) {
		t.Fatalf("failed build affected subsequent query: %q %#v", q, args)
	}
}

func TestCloneAppendAndReset(t *testing.T) {
	base := Select("id").From("t").GroupBy("id").GroupBy("v").Where(Eq("id", 1))
	derived := base.Clone().Select("v").Where(Eq("v", 2))

	checkSQL(t, base, "SELECT `id` FROM `t` WHERE (`id` = ?) GROUP BY `id`, `v`", 1)
	checkSQL(t, derived, "SELECT `id`, `v` FROM `t` WHERE ((`id` = ?) AND (`v` = ?)) GROUP BY `id`, `v`", 1, 2)
	checkSQL(t, derived.ClearWhere().ClearGroupBy().ClearSelect().Select("v"), "SELECT `v` FROM `t`")

	db := &DB{Dialect: dialect.Postgres}
	b := db.Select("id").Limit(-1)
	checkBuildError(t, b)
	checkSQL(t, b.Reset().SelectExpr(Expr("1")), "SELECT 1")

	sub := Select("id").From("t").Where(Eq("id", 1))
	q := Select().SelectExpr(Subquery(sub))
	sub.Where(Eq("id", 2))
	checkSQL(t, q, "SELECT (SELECT `id` FROM `t` WHERE (`id` = ?))", 1)

	input := []any{1}
	insert := Insert().Into("t").Values(input...)
	input[0] = 2
	other := insert.Clone().Values(3)
	checkSQL(t, insert, "INSERT INTO `t` VALUES (?)", 1)
	checkSQL(t, other, "INSERT INTO `t` VALUES (?), (?)", 1, 3)
}

func TestBuildErrorsAreNotExecuted(t *testing.T) {
	calls := 0
	r := recordingExecutor{call: func(string, []any) { calls++ }}
	b := Insert().Into("t").SetExecutor(r)
	if _, e := b.ExecContext(context.Background()); e == nil {
		t.Fatal("missing values")
	}
	if calls != 0 {
		t.Fatal("invalid SQL reached executor")
	}

	checkSQL(t, b.Values(1).SetDialect(dialect.Postgres), `INSERT INTO "t" VALUES ($1)`, 1)
	checkBuildError(t, Select().SelectExpr(Ident()))
	checkBuildError(t, Select("*").From("t").Where(InQuery("id", nil)))
}

func TestStatementCommentRejectsNestedDelimiter(t *testing.T) {
	checkBuildError(t, Select("id").Comment("nested /* comment").SetDialect(dialect.Postgres))
}

func TestConcurrentBuildOwnsNestedResults(t *testing.T) {
	q := Select("id").With("q", Select("id").From("t").Where(Eq("v", 1))).From("q").
		Where(Or(InQuery("id", Select("id").From("s").Where(Eq("v", 2))), Eq("id", 3))).
		SetDialect(dialect.Postgres)
	want := `WITH "q" AS (SELECT "id" FROM "t" WHERE ("v" = $1)) SELECT "id" FROM "q" WHERE ("id" IN (SELECT "id" FROM "s" WHERE ("v" = $2)) OR ("id" = $3))`
	for range 8 {
		t.Run("build", func(t *testing.T) {
			t.Parallel()
			for range 50 {
				checkSQL(t, q, want, 1, 2, 3)
			}
		})
	}
}
