// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"database/sql"
	"database/sql/driver"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/xgfone/go-sqlx/dialect"
)

func TestRepeatedExpressionParameters(t *testing.T) {
	e := Coalesce(Ident("v"), 0)
	q := Select().SelectExpr(e, Count("*")).From("t").Where(Gt("v", -1)).
		GroupByExpr(e).Having(Gt(e, 0)).OrderByExpr(e, Asc)

	for _, d := range []Dialect{dialect.Postgres, dialect.WithVersion(dialect.Postgres, 14, 0, 0)} {
		checkSQL(t, q.Clone().SetDialect(d),
			`SELECT COALESCE("v", $1), COUNT(*) FROM "t" WHERE ("v" > $2) GROUP BY COALESCE("v", $1) HAVING (COALESCE("v", $1) > $3) ORDER BY COALESCE("v", $1) ASC`, 0, -1, 0)
		checkSQL(t, Select().SelectExpr(e).From("t").Distinct().OrderByExpr(e, Desc).SetDialect(d),
			`SELECT DISTINCT COALESCE("v", $1) FROM "t" ORDER BY COALESCE("v", $1) DESC`, 0)
		checkSQL(t, Select().SelectExpr(Cast(e, "TEXT"), Count("*")).From("t").
			GroupByExpr(GroupingSets(GroupingSet(e), GroupingSet())).SetDialect(d),
			`SELECT CAST(COALESCE("v", $1) AS TEXT), COUNT(*) FROM "t" GROUP BY GROUPING SETS ((COALESCE("v", $1)), ())`, 0)
	}

	checkSQL(t, q.Clone().SetDialect(dialect.SQLite),
		`SELECT COALESCE("v", ?), COUNT(*) FROM "t" WHERE ("v" > ?) GROUP BY COALESCE("v", ?) HAVING (COALESCE("v", ?) > ?) ORDER BY COALESCE("v", ?) ASC`, 0, -1, 0, 0, 0, 0)
	checkSQL(t, q.Clone().SetDialect(dialect.MySQL),
		"SELECT COALESCE(`v`, ?), COUNT(*) FROM `t` WHERE (`v` > ?) GROUP BY COALESCE(`v`, ?) HAVING (COALESCE(`v`, ?) > ?) ORDER BY COALESCE(`v`, ?) ASC", 0, -1, 0, 0, 0, 0)

	// Equal constants in different expressions do not imply expression identity.
	other := Coalesce(Ident("w"), 0)
	checkSQL(t, Select().SelectExpr(e, other).From("t").GroupByExpr(e, other).SetDialect(dialect.Postgres),
		`SELECT COALESCE("v", $1), COALESCE("w", $2) FROM "t" GROUP BY COALESCE("v", $1), COALESCE("w", $2)`, 0, 0)
	// The same expression in a subquery gets that query's own bindings.
	sub := Select().SelectExpr(e).From("other").Distinct().OrderByExpr(e, Asc).Limit(1)
	checkSQL(t, Select().SelectExpr(e, Subquery(sub)).From("t").GroupByExpr(e).SetDialect(dialect.Postgres),
		`SELECT COALESCE("v", $1), (SELECT DISTINCT COALESCE("v", $2) FROM "other" ORDER BY COALESCE("v", $2) ASC LIMIT 1) FROM "t" GROUP BY COALESCE("v", $1)`, 0, 0)

	e = Coalesce(Ident("v"), Param(0))
	tmpl, err := Select().SelectExpr(e, Count("*")).From("t").GroupByExpr(e).
		Having(Gt(e, Param(1))).SetDialect(dialect.Postgres).Compile()
	if err != nil {
		t.Fatal(err)
	}

	for _, fallback := range []int{0, 7} {
		query, args, err := tmpl.Bind(fallback, 1)
		if err != nil || query != `SELECT COALESCE("v", $1), COUNT(*) FROM "t" GROUP BY COALESCE("v", $1) HAVING (COALESCE("v", $1) > $2)` || !reflect.DeepEqual(args, []any{fallback, 1}) {
			t.Fatalf("compiled reuse: %q %#v %v", query, args, err)
		}
	}

	// Sharing a runtime input across different type contexts must not unify its
	// placeholders just because both expressions contain the same Param.
	p := Param(0)
	left, right := Coalesce(Ident("n"), p), Coalesce(Ident("s"), p)
	tmpl, err = Select().SelectExprAlias(left, "a").SelectExprAlias(right, "b").From("t").
		Distinct().OrderByAsc("a").SetDialect(dialect.Postgres).Compile()
	if err != nil {
		t.Fatal(err)
	}

	query, args, err := tmpl.Bind("2")
	if err != nil || query != `SELECT DISTINCT COALESCE("n", $1) AS "a", COALESCE("s", $2) AS "b" FROM "t" ORDER BY "a" ASC` || !reflect.DeepEqual(args, []any{"2", "2"}) {
		t.Fatalf("independent parameter type contexts: %q %#v %v", query, args, err)
	}

	v := Value("2")
	checkSQL(t, Select().SelectExpr(Coalesce(Ident("n"), v), Coalesce(Ident("s"), v)).From("t").Distinct().OrderByExpr(Expr("1"), Asc).SetDialect(dialect.Postgres),
		`SELECT DISTINCT COALESCE("n", $1), COALESCE("s", $2) FROM "t" ORDER BY 1 ASC`, "2", "2")
	checkSQL(t, Select().SelectExpr(v).Distinct().OrderByExpr(v, Asc).SetDialect(dialect.Postgres),
		`SELECT DISTINCT $1 ORDER BY $1 ASC`, "2")
}

func TestScalarSubqueriesInReturningAndLockedSelect(t *testing.T) {
	e := Coalesce(Subquery(Select().SelectExpr(Sum("v")).From("other")), 0)
	checkSQL(t, Update().Table("t").Set(Set("v", 1)).ReturningExpr(e, "total").SetDialect(dialect.Postgres),
		`UPDATE "t" SET "v"=$1 RETURNING COALESCE((SELECT SUM("v") FROM "other"), $2) AS "total"`, 1, 0)
	checkSQL(t, Select().SelectExpr(e).From("t").ForUpdate().SetDialect(dialect.Postgres),
		`SELECT COALESCE((SELECT SUM("v") FROM "other"), $1) FROM "t" FOR UPDATE`, 0)
}

func TestSQLiteReturningQualification(t *testing.T) {
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

type valuesAuditValuer struct{ calls *int }

func (v valuesAuditValuer) Value() (driver.Value, error) { *v.calls++; return int64(2), nil }

func TestValuesSourceTypes(t *testing.T) {
	type signed int32
	type bytes []byte
	stamp := time.Date(2026, 9, 13, 0, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		value any
		typ   string
	}{
		{2, "BIGINT"}, {signed(2), "BIGINT"}, {uint64(2), "NUMERIC"},
		{2.5, "DOUBLE PRECISION"}, {true, "BOOLEAN"}, {"2", "TEXT"},
		{bytes{1, 2}, "BYTEA"}, {stamp, "TIMESTAMP WITH TIME ZONE"},
		{sql.Named("v", 2), "BIGINT"}, {Value(2), "BIGINT"},
	} {
		q, _, err := Select("*").FromSource(ValuesSource("d", []string{"v"}, []any{tc.value})).
			SetDialect(dialect.Postgres).Build()
		if err != nil || !strings.Contains(q, "CAST($1 AS "+tc.typ+")") {
			t.Fatalf("type %T: %q %v", tc.value, q, err)
		}
	}

	source := ValuesSource("d", []string{"v"}, []any{nil}, []any{Value(2)})
	checkSQL(t, Select("*").FromSource(source).SetDialect(dialect.Postgres),
		`SELECT * FROM (VALUES ($1), (CAST($2 AS BIGINT))) AS "d" ("v")`, nil, 2)

	types := []string{"INTEGER"}
	typed := source.ColumnTypes(types...)
	types[0] = "TEXT"
	checkSQL(t, Select("*").FromSource(typed).SetDialect(dialect.SQLite),
		`SELECT * FROM (SELECT CAST(? AS INTEGER) AS "v" UNION ALL SELECT CAST(? AS INTEGER)) AS "d"`, nil, 2)
	checkSQL(t, Select("*").FromSource(typed.ColumnTypes()).SetDialect(dialect.SQLite),
		`SELECT * FROM (SELECT ? AS "v" UNION ALL SELECT ?) AS "d"`, nil, 2)

	for _, s := range []Source{
		source.ColumnTypes(""),
		source.ColumnTypes("INTEGER", "TEXT"),
		TableSource("t", "").ColumnTypes("INTEGER"),
	} {
		checkBuildError(t, Select("*").FromSource(s).SetDialect(dialect.Postgres))
	}

	calls := 0
	v := valuesAuditValuer{&calls}
	checkBuildError(t, Select("*").FromSource(ValuesSource("d", []string{"v"}, []any{v})).SetDialect(dialect.Postgres))

	q := Select("*").FromSource(ValuesSource("d", []string{"v"}, []any{v}, []any{Param(0)}).
		ColumnTypes("INTEGER")).SetDialect(dialect.Postgres)
	tmpl, err := q.Compile()
	if err != nil {
		t.Fatal(err)
	}

	s, args, err := tmpl.Bind(10)
	if err != nil || calls != 0 || len(args) != 2 || args[0] != v || args[1] != 10 ||
		s != `SELECT * FROM (VALUES (CAST($1 AS INTEGER)), (CAST($2 AS INTEGER))) AS "d" ("v")` {
		t.Fatalf("typed template: %q %#v %v, calls=%d", s, args, err, calls)
	}

	_, err = Select("*").FromSource(ValuesSource("d", []string{"v"}, []any{Param(0)})).
		SetDialect(dialect.Postgres).Compile()
	if err == nil {
		t.Fatal("untyped VALUES Param accepted")
	}
}

func TestMySQLRollupVersionCombinations(t *testing.T) {
	for _, patch := range []int{0, 11, 12, 14, 31} {
		d := dialect.WithVersion(dialect.MySQL, 8, 0, patch)
		base := Select("v").SelectExpr(Count("*")).From("t").GroupByRollup("v").SetDialect(d)
		for _, q := range []*SelectBuilder{base.Clone().Distinct(), base.Clone().OrderByAsc("v")} {
			_, _, err := q.Build()
			if (err != nil) != (patch < 12) {
				t.Fatalf("8.0.%d: %v", patch, err)
			}
		}
		if _, _, err := base.Build(); err != nil {
			t.Fatal(err)
		}
	}

	checkBuildError(t, Select("v").From("t").GroupByRollup("v").OrderByAsc("v").SetDialect(dialect.MySQL))

	// Global compound ordering does not order the ROLLUP operand.
	checkSQL(t,
		Select("v").From("t").GroupByRollup("v").Union(Select("v").From("other")).OrderByAsc("v").SetDialect(dialect.MySQL),
		"SELECT `v` FROM `t` GROUP BY `v` WITH ROLLUP UNION SELECT `v` FROM `other` ORDER BY `v` ASC")
	checkSQL(t,
		Select("v").From("t").Distinct().GroupByRollup("v").OrderByAsc("v").SetDialect(dialect.Postgres),
		`SELECT DISTINCT "v" FROM "t" GROUP BY ROLLUP ("v") ORDER BY "v" ASC`)
}
