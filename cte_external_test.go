// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx_test

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/xgfone/go-sqlx"
	"github.com/xgfone/go-sqlx/dialect"
)

// This application type has no Build method or embedded sqlx implementation.
type customCTEBody struct{ values []any }

func (b *customCTEBody) Kind() sqlx.CTEBodyKind {
	return sqlx.CTESelect
}

func (b *customCTEBody) Snapshot() sqlx.CTEBody {
	return &customCTEBody{values: slices.Clone(b.values)}
}

func (b *customCTEBody) WriteSQL(buf *strings.Builder, ctx *sqlx.BuildContext) error {
	buf.WriteString("SELECT ")
	for i, value := range b.values {
		if i != 0 {
			buf.WriteString(", ")
		}
		ctx.WriteValue(buf, value)
	}
	return nil
}

var _ sqlx.CTEBody = (*customCTEBody)(nil)

func ExampleCTEBody() {
	body := &customCTEBody{values: []any{7}}
	cte := sqlx.NewCTE("input", body, "id")
	body.values[0] = 99 // The CTE retains its own snapshot.
	query, args, err := sqlx.Select("id").From("input").WithCTE(cte).
		Where(sqlx.Gt("id", 3)).SetDialect(dialect.Postgres).Build()
	if err != nil {
		fmt.Println(err)
		return
	}

	fmt.Println(query)
	fmt.Println(args)
	// Output:
	// WITH "input" ("id") AS (SELECT $1) SELECT "id" FROM "input" WHERE ("id" > $2)
	// [7 3]
}

func TestExternalCTEBodyBindingsAndSnapshot(t *testing.T) {
	body := &customCTEBody{values: []any{
		sqlx.Subquery(sqlx.Select().SelectExpr(sqlx.Value(20)).SetDialect(dialect.MySQL)),
		sqlx.Expr("? + ?", 30, 31),
	}}

	if _, ok := any(body).(sqlx.SQLBuilder); ok {
		t.Fatal("test body must exercise composition without a Build method")
	}

	columns := []string{"id", "v"}
	cte := sqlx.NewCTE("input", body, columns...)
	body.values[0], columns[0] = 999, "changed"
	query := sqlx.Select().SelectExpr(sqlx.Value(40)).From("input").
		With("first", sqlx.Select().SelectExpr(sqlx.Value(10))).WithCTE(cte).
		Where(sqlx.Gt("id", 50))

	for _, tc := range []struct {
		dialect sqlx.Dialect
		want    string
	}{
		{
			dialect.Postgres,
			`WITH "first" AS (SELECT $1), "input" ("id", "v") AS (SELECT (SELECT $2), $3 + $4) SELECT $5 FROM "input" WHERE ("id" > $6)`,
		},
		{
			dialect.SQLite,
			`WITH "first" AS (SELECT ?), "input" ("id", "v") AS (SELECT (SELECT ?), ? + ?) SELECT ? FROM "input" WHERE ("id" > ?)`,
		},
		{
			dialect.MySQL,
			"WITH `first` AS (SELECT ?), `input` (`id`, `v`) AS (SELECT (SELECT ?), ? + ?) SELECT ? FROM `input` WHERE (`id` > ?)",
		},
	} {
		t.Run(tc.dialect.Name(), func(t *testing.T) {
			q := query.Clone().SetDialect(tc.dialect)
			for range 4 {
				t.Run("concurrent", func(t *testing.T) {
					t.Parallel()
					for range 20 {
						assertSQL(t, q, tc.want, 10, 20, 30, 31, 40, 50)
					}
				})
			}
		})
	}
}

func TestExternalCTEBodyNamedParameters(t *testing.T) {
	value := sql.Named("id", 7)
	body := &customCTEBody{values: []any{value, value}}
	q := sqlx.Select().SelectExpr(sqlx.Value(value)).
		WithCTE(sqlx.NewCTE("q", body, "a", "b")).SetDialect(dialect.SQLite)
	assertSQL(t, q, `WITH "q" ("a", "b") AS (SELECT @id, @id) SELECT @id`, value)

	q.ClearSelect().SelectExpr(sqlx.Value(sql.Named("id", 8)))
	if _, _, err := q.Build(); err == nil {
		t.Fatal("conflicting names across CTE and parent must fail")
	}
}

type wrappedCTEBody struct{ *sqlx.SelectBuilder }

func (b wrappedCTEBody) Snapshot() sqlx.CTEBody {
	return wrappedCTEBody{b.Clone()}
}

func (b wrappedCTEBody) WriteSQL(buf *strings.Builder, ctx *sqlx.BuildContext) error {
	buf.WriteString("/* wrapper */ ")
	return b.SelectBuilder.WriteSQL(buf, ctx)
}

func TestExternalCTEBodyWrapperPreservesRendering(t *testing.T) {
	body := wrappedCTEBody{sqlx.Select().SelectExpr(sqlx.Value(7))}
	cte := sqlx.NewCTE("q", body, "id")
	body.ClearSelect().SelectExpr(sqlx.Value(99))
	assertSQL(
		t,
		sqlx.Select("id").From("q").WithCTE(cte).SetDialect(dialect.Postgres),
		`WITH "q" ("id") AS (/* wrapper */ SELECT $1) SELECT "id" FROM "q"`,
		7,
	)
}

type delegatedCTEBody struct{ body sqlx.CTEBody }

func (b delegatedCTEBody) Kind() sqlx.CTEBodyKind {
	return b.body.Kind()
}
func (b delegatedCTEBody) Snapshot() sqlx.CTEBody {
	return delegatedCTEBody{b.body.Snapshot()}
}
func (b delegatedCTEBody) WriteSQL(buf *strings.Builder, ctx *sqlx.BuildContext) error {
	return b.body.WriteSQL(buf, ctx)
}

func TestExternalCTEBodyKindsAndPlacement(t *testing.T) {
	for _, body := range []sqlx.CTEBody{
		sqlx.Insert().Into("t").Columns("id").Values(7).Returning("id"),
		sqlx.Update().Table("t").Set(sqlx.Set("id", 7)).Returning("id"),
		sqlx.Delete().From("t").Where(sqlx.Eq("id", 7)).Returning("id"),
	} {
		cte := sqlx.NewCTE("q", delegatedCTEBody{body})
		q := sqlx.Select("id").From("q").WithCTE(cte).SetDialect(dialect.Postgres)
		var buf strings.Builder
		ctx := sqlx.NewBuildContext(dialect.Postgres)
		if err := body.WriteSQL(&buf, ctx); err != nil {
			t.Fatal(err)
		}

		assertSQL(t, q, `WITH "q" AS (`+buf.String()+`) SELECT "id" FROM "q"`, 7)
		for _, invalid := range []sqlx.SQLBuilder{
			q.Clone().SetDialect(dialect.SQLite),
			q.Clone().SetDialect(dialect.MySQL),
			q.Clone().ClearWith().WithCTE(cte.Materialized()),
			q.Clone().ClearWith().WithCTE(cte.NotMaterialized()),
			sqlx.Select("*").FromSelect(q, "nested").SetDialect(dialect.Postgres),
			sqlx.Select("*").WithCTE(sqlx.NewCTE("nested", delegatedCTEBody{q})).SetDialect(dialect.Postgres),
		} {
			if _, _, err := invalid.Build(); err == nil {
				t.Fatalf("expected invalid CTE placement or dialect: %T", invalid)
			}
		}
	}

	cte := sqlx.NewCTE("q", &customCTEBody{values: []any{7}}, "id")
	assertSQL(t,
		sqlx.Select("id").From("q").WithCTE(cte.Materialized()).SetDialect(dialect.SQLite),
		`WITH "q" ("id") AS MATERIALIZED (SELECT ?) SELECT "id" FROM "q"`,
		7)
	assertSQL(t,
		sqlx.Insert().Into("t").Columns("id").WithCTE(cte).FromSelect(sqlx.Select("id").From("q")),
		"INSERT INTO `t` (`id`) WITH `q` (`id`) AS (SELECT ?) SELECT `id` FROM `q`",
		7)
}

type failingCTEBody struct {
	kind     sqlx.CTEBodyKind
	snapshot func() sqlx.CTEBody
	write    func(*strings.Builder, *sqlx.BuildContext) error
}

func (b failingCTEBody) Kind() sqlx.CTEBodyKind {
	return b.kind
}
func (b failingCTEBody) Snapshot() sqlx.CTEBody {
	if b.snapshot != nil {
		return b.snapshot()
	}
	return b
}
func (b failingCTEBody) WriteSQL(buf *strings.Builder, ctx *sqlx.BuildContext) error {
	if b.write != nil {
		return b.write(buf, ctx)
	}
	return nil
}

func TestExternalCTEBodyFailures(t *testing.T) {
	failure := errors.New("custom CTE failure")
	for _, tc := range []struct {
		name string
		body sqlx.CTEBody
		want string
	}{
		{"nil", nil, "nil CTE"},
		{"nil custom pointer", (*customCTEBody)(nil), "nil CTE"},
		{"nil select", (*sqlx.SelectBuilder)(nil), "nil CTE"},
		{"nil insert", (*sqlx.InsertBuilder)(nil), "nil CTE"},
		{"nil update", (*sqlx.UpdateBuilder)(nil), "nil CTE"},
		{"nil delete", (*sqlx.DeleteBuilder)(nil), "nil CTE"},
		{"zero kind", failingCTEBody{}, "invalid CTE body kind"},
		{"unknown kind", failingCTEBody{kind: 255}, "invalid CTE body kind"},
		{"empty body", failingCTEBody{kind: sqlx.CTESelect}, "empty CTE body"},
		{"nil snapshot", failingCTEBody{snapshot: func() sqlx.CTEBody { return nil }}, "nil CTE"},
		{"typed nil snapshot", failingCTEBody{snapshot: func() sqlx.CTEBody { return (*customCTEBody)(nil) }}, "nil CTE"},
		{"returned error", failingCTEBody{kind: sqlx.CTESelect, write: func(buf *strings.Builder, ctx *sqlx.BuildContext) error {
			buf.WriteString("SELECT ")
			ctx.WriteArg(buf, 99)
			return failure
		}}, failure.Error()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			q := sqlx.Select("*").WithCTE(sqlx.NewCTE("q", tc.body)).SetDialect(dialect.Postgres)
			query, args, err := q.Build()
			if err == nil || !strings.Contains(err.Error(), tc.want) || query != "" || args != nil {
				t.Fatalf("got %q %#v %v; want error containing %q and no partial output",
					query, args, err, tc.want)
			}
			if tc.want == failure.Error() && !errors.Is(err, failure) {
				t.Fatalf("error wrapping lost the original cause: %v", err)
			}

			assertSQL(t, sqlx.Select().SelectExpr(sqlx.Value(7)).SetDialect(dialect.Postgres), `SELECT $1`, 7)
		})
	}
}

func TestBuiltinCTEBodyWriteSQL(t *testing.T) {
	for _, tc := range []struct {
		body sqlx.CTEBody
		want string
	}{
		{sqlx.Select().SelectExpr(sqlx.Value(7)), `SELECT $2`},
		{sqlx.Insert().Into("t").Values(7), `INSERT INTO "t" VALUES ($2)`},
		{sqlx.Update().Table("t").Set(sqlx.Set("id", 7)), `UPDATE "t" SET "id"=$2`},
		{sqlx.Delete().From("t").Where(sqlx.Eq("id", 7)), `DELETE FROM "t" WHERE ("id" = $2)`},
	} {
		var buf strings.Builder
		buf.WriteString("prefix: ")
		ctx := sqlx.NewBuildContext(dialect.Postgres)
		ctx.Add(0)
		if err := tc.body.WriteSQL(&buf, ctx); err != nil {
			t.Fatal(err)
		}
		if buf.String() != "prefix: "+tc.want || !reflect.DeepEqual(ctx.Args(), []any{0, 7}) {
			t.Fatalf("shared rendering: %q %#v", buf.String(), ctx.Args())
		}
	}

	for _, body := range []sqlx.CTEBody{
		sqlx.Select(),
		sqlx.Insert(),
		sqlx.Update(),
		sqlx.Delete(),
		(*sqlx.SelectBuilder)(nil),
	} {
		var buf strings.Builder
		if err := body.WriteSQL(&buf, sqlx.NewBuildContext(dialect.Postgres)); err == nil {
			t.Fatalf("expected validation error for %T", body)
		}
	}
}

func TestBuildContextPublicWriters(t *testing.T) {
	ctx := sqlx.NewBuildContext(dialect.Postgres)
	var buf strings.Builder
	ctx.WriteQuote(&buf, `t.a"b`)
	buf.WriteByte('=')
	ctx.WriteArg(&buf, 7)
	buf.WriteString(" AND ")
	ctx.WriteValue(&buf, sqlx.Expr("? > ?", sqlx.Ident("t", "id"), 8))
	if buf.String() != `"t"."a""b"=$1 AND "t"."id" > $2` ||
		!reflect.DeepEqual(ctx.Args(), []any{7, 8}) {
		t.Fatalf("streaming helpers: %q %#v", buf.String(), ctx.Args())
	}
}

func TestExternalCTEBodySQLiteExecution(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 unavailable")
	}

	query, args, err := sqlx.Select("id").From("input").WithCTE(
		sqlx.NewCTE("input", &customCTEBody{values: []any{42}}, "id").Materialized(),
	).Where(sqlx.Gt("id", 7)).SetDialect(dialect.SQLite).Build()
	if err != nil {
		t.Fatal(err)
	}

	params, err := json.Marshal(args)
	if err != nil {
		t.Fatal(err)
	}

	result, err := exec.Command(python, "-c",
		`import json, sqlite3, sys; print(json.dumps(sqlite3.connect(":memory:").execute(sys.argv[1], json.loads(sys.argv[2])).fetchall()))`,
		query, string(params)).CombinedOutput()
	if err != nil || strings.TrimSpace(string(result)) != "[[42]]" {
		t.Fatalf("SQLite CTE execution: %s (%v)", result, err)
	}
}

func BenchmarkExternalCTEBody(b *testing.B) {
	ctebody := &customCTEBody{values: []any{7}}
	query := sqlx.Select("id").From("input").
		WithCTE(sqlx.NewCTE("input", ctebody, "id")).
		SetDialect(dialect.Postgres)

	b.ReportAllocs()
	for b.Loop() {
		if _, _, err := query.Build(); err != nil {
			b.Fatal(err)
		}
	}
}
