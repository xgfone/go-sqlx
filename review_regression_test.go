// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"context"
	"database/sql"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/xgfone/go-sqlx/dialect"
)

func TestExpressionDialectQuoteBoundaries(t *testing.T) {
	checkSQL(t, Select().SelectExpr(Expr("[why?] + ?", 2)).SetDialect(dialect.SQLite),
		`SELECT [why?] + ?`, 2)
	checkSQL(t, Select().SelectExpr(Expr("$标签_1$? ' /*$标签_1$ || ?", "x")).SetDialect(dialect.Postgres),
		`SELECT $标签_1$? ' /*$标签_1$ || $1`, "x")
	checkSQL(t, Select().SelectExpr(Expr("ARRAY[?]", 2)).SetDialect(dialect.Postgres),
		`SELECT ARRAY[$1]`, 2)
	checkBuildError(t, Select().SelectExpr(Expr("[unclosed? + ?", 2)).SetDialect(dialect.SQLite))
	checkBuildError(t, Select().SelectExpr(Expr("$标签$?", 2)).SetDialect(dialect.Postgres))
}

func TestStatementCommentRejectsNestedDelimiter(t *testing.T) {
	checkBuildError(t, Select("id").Comment("nested /* comment").SetDialect(dialect.Postgres))
}

type nilRowsExecutor struct{ Executor }

func (nilRowsExecutor) QueryContext(context.Context, string, ...any) (*sql.Rows, error) {
	return nil, nil
}

func TestBuilderQueryRejectsNilExecutorRows(t *testing.T) {
	db := &DB{Dialect: dialect.Postgres, Executor: nilRowsExecutor{}}
	for _, err := range []error{
		db.Select("id").QueryRowsContext(context.Background()).Err(),
		db.Select("id").QueryRowContext(context.Background()).Err(),
		db.Insert().Into("t").Values(1).Returning("id").QueryRowsContext(context.Background()).Err(),
	} {
		if err == nil || !strings.Contains(err.Error(), "nil rows") {
			t.Fatalf("expected nil rows error, got %v", err)
		}
	}
}

func TestPredicateDefinedStringOperands(t *testing.T) {
	type column string
	const name column = "t.id"
	checkSQL(t, Select("id").Where(Eq(name, 1), Between(name, 2, 3), In(name, 4, 5)).SetDialect(dialect.Postgres),
		`SELECT "id" WHERE (("t"."id" = $1) AND ("t"."id" BETWEEN $2 AND $3) AND ("t"."id" IN ($4, $5)))`,
		1, 2, 3, 4, 5)
	checkSQL(t, Select().SelectExpr(Value(1)).Where(Eq(Value(2), 3)).SetDialect(dialect.Postgres),
		`SELECT $1 WHERE ($2 = $3)`,
		1, 2, 3)
}

func TestConnURLLocationEscaping(t *testing.T) {
	loc := time.FixedZone("local+08&timeout=1", 8*60*60)
	for _, connURL := range []string{"user@/db", "user@/db?charset=utf8"} {
		got := SetConnURLLocation(connURL, loc)
		_, query, _ := strings.Cut(got, "?")
		values, err := url.ParseQuery(query)
		if err != nil || values.Get("loc") != loc.String() || values.Has("timeout") {
			t.Fatalf("location was not escaped: %q (%v)", got, err)
		}
	}
}

func TestStreamingCustomConditionsCalledOnce(t *testing.T) {
	calls := 0
	custom := ConditionFunc(func(c *BuildContext) string {
		calls++
		return c.Add(7) + "=7"
	})
	checkSQL(t, Select("id").Where(Or(nil, And(), custom), Eq("v", 8)).SetDialect(dialect.Postgres),
		`SELECT "id" WHERE ($1=7 AND ("v" = $2))`, 7, 8)
	if calls != 1 {
		t.Fatalf("custom condition evaluated %d times", calls)
	}
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

type customPaginationDialect struct{ Dialect }

func (customPaginationDialect) LimitOffset(dialect.Pagination) string {
	return "FETCH FIRST 2 ROWS ONLY"
}

func TestStreamingPaginationPreservesCustomDialect(t *testing.T) {
	custom := customPaginationDialect{dialect.Postgres}
	for _, d := range []Dialect{custom, dialect.WithFeatures(custom, nil, nil)} {
		checkSQL(t, Select("id").Limit(2).SetDialect(d), `SELECT "id" FETCH FIRST 2 ROWS ONLY`)
	}
}

func TestInsertCapacityEstimationDoesNotEvaluateExpressions(t *testing.T) {
	calls := 0
	e := Expression{
		node: &expressionWriter{
			write: func(s *strings.Builder, c *BuildContext) {
				calls++
				c.writeArg(s, 1)
			},
		},
	}

	values := make([]any, 128)
	for i := range values {
		values[i] = e
	}

	_, args, err := Insert().Into("t").Values(values...).Build()
	if err != nil || len(args) != len(values) || calls != len(values) {
		t.Fatalf("expression evaluation: calls=%d args=%d err=%v", calls, len(args), err)
	}
}

func BenchmarkBuildNestedPredicates(b *testing.B) {
	q := Select("id").From("t").Where(
		Or(Eq("a", 1), Eq("b", 2)),
		Or(Between("c", 3, 4), In("d", 5, 6, 7)),
		InQuery("id", Select("id").From("s").Where(Gt("v", 8))),
	).SetDialect(dialect.Postgres)
	b.ReportAllocs()
	for b.Loop() {
		s, _, err := q.Build()
		if err != nil {
			b.Fatal(err)
		}
		expressionHelperSQL = s
	}
}
