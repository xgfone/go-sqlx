// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"database/sql"
	"strconv"
	"strings"
	"testing"

	"github.com/xgfone/go-sqlx/dialect"
)

type streamingTestDialect struct{ Dialect }

func (streamingTestDialect) Placeholder(i int) string { return ":" + strconv.Itoa(i) }
func (streamingTestDialect) QuoteIdent(name string) string {
	return "[" + strings.ReplaceAll(name, "]", "]]") + "]"
}

func TestStreamingExpressionBindingsAndCustomDialect(t *testing.T) {
	q := Select().SelectExpr(CaseValue(Value(10)).WhenValue(Value(20), Expr("? + ?", 30, 40)).Else(Value(50)).End()).
		Where(Eq("t.key]", 60), Expr("? > ?", 70, 80).Condition())
	for _, d := range []Dialect{dialect.Postgres, dialect.WithVersion(dialect.Postgres, 14, 0, 0)} {
		checkSQL(t, q.Clone().SetDialect(d),
			`SELECT CASE $1 WHEN $2 THEN $3 + $4 ELSE $5 END WHERE (("t"."key]" = $6) AND ($7 > $8))`,
			10, 20, 30, 40, 50, 60, 70, 80)
	}

	custom := streamingTestDialect{dialect.Postgres}
	for _, d := range []Dialect{custom, dialect.WithFeatures(custom, nil, nil)} {
		checkSQL(t, q.Clone().SetDialect(d),
			`SELECT CASE :1 WHEN :2 THEN :3 + :4 ELSE :5 END WHERE (([t].[key]]] = :6) AND (:7 > :8))`,
			10, 20, 30, 40, 50, 60, 70, 80)
	}
}

func TestStreamingNativeAndCustomClauses(t *testing.T) {
	empty := ConditionWriterFunc(func(*SQLWriter) (bool, error) { return false, nil })
	custom := ConditionWriterFunc(func(w *SQLWriter) (bool, error) {
		w.Path("a")
		w.Raw("=")
		w.Arg(1)
		return true, nil
	})
	checkSQL(t,
		Select().SelectExpr(Value(0)).Where(Or(empty, custom, Eq("b", 2))).SetDialect(dialect.Postgres),
		`SELECT $1 WHERE ("a"=$2 OR ("b" = $3))`, 0, 1, 2)
	checkSQL(t,
		Select().SelectExpr(Value(0)).Where(Or(empty, Eq("b", 2))).SetDialect(dialect.Postgres),
		`SELECT $1 WHERE ("b" = $2)`, 0, 2)
	checkBuildError(t, Select().SelectExpr(Case().When(empty, 1).End()))

	set := UpdaterWriterFunc(func(w *SQLWriter) (bool, error) {
		w.Path("b")
		w.Raw("=")
		w.Arg(2)
		return true, nil
	})
	emptySet := UpdaterWriterFunc(func(*SQLWriter) (bool, error) { return false, nil })
	checkSQL(t,
		Update().Table("t").Set(Batch(emptySet, Set("a", 1), set), Set("c", 3)).SetDialect(dialect.Postgres),
		`UPDATE "t" SET "a"=$1, "b"=$2, "c"=$3`, 1, 2, 3)
	checkBuildError(t, Update().Table("t").Set(Batch(emptySet)))
}

func TestStreamingNamedArgumentReuse(t *testing.T) {
	match, result := sql.Named("match", 1), sql.Named("result", "yes")
	e := CaseValue(match).WhenValue(match, result).Else(result).End()
	for _, d := range []Dialect{dialect.SQLite, dialect.WithFeatures(dialect.SQLite, nil, nil)} {
		checkSQL(t,
			Select().SelectExpr(e, Value(9)).SetDialect(d),
			`SELECT CASE @match WHEN @match THEN @result ELSE @result END, ?`, match, result, 9)
		checkBuildError(t, Select().SelectExpr(CaseValue(match).WhenValue(sql.Named("match", 2), 3).End()).SetDialect(d))
	}
}

func TestRenderingBufferReentrancyAndLifetime(t *testing.T) {
	custom := ConditionWriterFunc(func(w *SQLWriter) (bool, error) {
		w.Raw(BuildCondition(w.ctx, Eq("a", 1)))
		return true, nil
	})

	q := Select().SelectExpr(Case().When(custom, 2).Else(3).End()).SetDialect(dialect.Postgres)
	first, c, err := buildBorrowed(q, &q.builderBase)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseBuildContext(c)

	second := BuildCondition(c, Between("b", 4, 5))
	if first != `SELECT CASE WHEN ("a" = $1) THEN $2 ELSE $3 END` || second != `("b" BETWEEN $4 AND $5)` {
		t.Fatalf("buffer reuse changed SQL: %q, %q", first, second)
	}

	func() {
		defer func() {
			if recover() == nil {
				t.Error("expected rendering to panic")
			}
		}()
		Case().When(ConditionWriterFunc(func(*SQLWriter) (bool, error) { return false, nil }), 0).End().render(c)
	}()

	if c.bufferInUse || c.buffer.Len() != 0 {
		t.Fatal("rendering panic retained the borrowed buffer")
	}
	if got := BuildCondition(c, Eq("c", 6)); got != `("c" = $6)` {
		t.Fatalf("rendering after panic: %q", got)
	}
	if first != `SELECT CASE WHEN ("a" = $1) THEN $2 ELSE $3 END` {
		t.Fatal("later rendering mutated a retained SQL string")
	}
}

func TestMixedSetOperationsKeepCTEScope(t *testing.T) {
	q := Select("v").WithColumns("q", Select().SelectExpr(Value(1)), "v").From("q").
		Union(Select("v").From("q")).Intersect(Select("v").From("q")).
		Except(Select("v").From("q")).OrderByAsc("v")
	checkSQL(t, q.Clone().SetDialect(dialect.Postgres),
		`WITH "q" ("v") AS (SELECT $1) ((SELECT "v" FROM "q" UNION SELECT "v" FROM "q") INTERSECT SELECT "v" FROM "q") EXCEPT SELECT "v" FROM "q" ORDER BY "v" ASC`, 1)
	checkSQL(t, q.Clone().SetDialect(dialect.SQLite),
		`WITH "q" ("v") AS (SELECT ?) SELECT * FROM (SELECT * FROM (SELECT "v" FROM "q" UNION SELECT "v" FROM "q") AS "_sqlx_set" INTERSECT SELECT "v" FROM "q") AS "_sqlx_set" EXCEPT SELECT "v" FROM "q" ORDER BY "v" ASC`, 1)
}
