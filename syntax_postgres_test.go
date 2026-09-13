// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xgfone/go-sqlx/dialect"
)

// Optional execution coverage using an externally installed PostgreSQL WASM
// engine. This keeps the Go module free of driver and JavaScript dependencies.
// SQLX_PGLITE_MODULE must name @electric-sql/pglite/dist/index.js.
func TestPostgresExpressionSemanticsExecution(t *testing.T) {
	module := os.Getenv("SQLX_PGLITE_MODULE")
	if module == "" {
		t.Skip("set SQLX_PGLITE_MODULE to run PostgreSQL execution tests")
	}

	module, err := filepath.Abs(module)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(module); err != nil {
		t.Fatal(err)
	}

	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal(err)
	}

	e := Coalesce(Ident("v"), 0)
	source := ValuesSource("d", []string{"v"}, []any{2}, []any{10})
	scalar := Coalesce(Subquery(Select().SelectExpr(Sum("v")).From("other")), 0)
	cases := []struct {
		name string
		b    SQLBuilder
		want [][]any
	}{
		{
			"group",
			Select().SelectExpr(e, Count("*")).From("t").GroupByExpr(e).
				OrderByExpr(e, Asc).SetDialect(dialect.Postgres),
			[][]any{{0, 1}, {2, 2}, {10, 1}},
		},
		{
			"distinct order",
			Select().SelectExpr(e).From("t").Distinct().OrderByExpr(e, Asc).
				SetDialect(dialect.Postgres),
			[][]any{{0}, {2}, {10}},
		},
		{
			"nested group expression",
			Select().SelectExpr(Cast(e, "TEXT"), Count("*")).From("t").
				GroupByExpr(e).OrderByExpr(e, Asc).SetDialect(dialect.Postgres),
			[][]any{{"0", 1}, {"2", 2}, {"10", 1}},
		},
		{
			"having",
			Select().SelectExpr(e, Count("*")).From("t").GroupByExpr(e).
				Having(Gt(e, 0)).OrderByExpr(e, Asc).SetDialect(dialect.Postgres),
			[][]any{{2, 2}, {10, 1}},
		},
		{
			"grouping sets",
			Select().SelectExpr(e, Count("*")).From("t").
				GroupByExpr(GroupingSets(GroupingSet(e), GroupingSet())).
				OrderByExpr(e, Asc).SetDialect(dialect.Postgres),
			[][]any{{0, 1}, {2, 2}, {10, 1}, {nil, 4}},
		},
		{
			"group alias with repeated projection and having",
			Select().SelectExprAlias(e, "a").SelectExprAlias(e, "b").From("t").
				GroupBy("a").Having(Gt(e, 0)).OrderByAsc("a").SetDialect(dialect.Postgres),
			[][]any{{2, 2}, {10, 10}},
		},
		{
			"rollup",
			Select().SelectExpr(e, Count("*")).From("t").GroupByRollupExpr(e).
				OrderByExpr(e, Asc).SetDialect(dialect.Postgres),
			[][]any{{0, 1}, {2, 2}, {10, 1}, {nil, 4}},
		},
		{
			"distinct on",
			Select().SelectExpr(e).From("t").DistinctOnExpr(e).
				OrderByExpr(e, Asc).SetDialect(dialect.Postgres),
			[][]any{{0}, {2}, {10}},
		},
		{
			"values numeric order",
			Select("d.v").FromSource(source).OrderByAsc("d.v").SetDialect(dialect.Postgres),
			[][]any{{2}, {10}},
		},
		{
			"values numeric compare",
			Select("d.v").FromSource(source).Where(Gt("d.v", 2)).SetDialect(dialect.Postgres),
			[][]any{{10}},
		},
		{
			"values numeric join",
			Select("t.v").From("t").JoinSource(InnerJoin, source, On("t.v", "d.v")).
				OrderByAsc("t.v").SetDialect(dialect.Postgres),
			[][]any{{2}, {2}, {10}},
		},
		{
			"values null and numeric",
			Select("d.v").FromSource(ValuesSource("d", []string{"v"}, []any{nil}, []any{2}, []any{10})).
				OrderByAsc("d.v").SetDialect(dialect.Postgres),
			[][]any{{2}, {10}, {nil}},
		},
		{
			"values float and boolean",
			Select("d.v", "d.b").FromSource(ValuesSource("d", []string{"v", "b"}, []any{2.5, true}, []any{10.5, false})).
				Where(Eq("d.b", true)).SetDialect(dialect.Postgres),
			[][]any{{2.5, true}},
		},
		{
			"returning aggregate subquery",
			Update().Table("t").Set(Set("v", 1)).Where(Eq("v", 10)).
				ReturningExpr(scalar, "total").SetDialect(dialect.Postgres),
			[][]any{{3}},
		},
		{
			"locked scalar subquery",
			Select().SelectExpr(scalar).From("t").Where(Eq("v", 10)).
				ForUpdate().SetDialect(dialect.Postgres),
			[][]any{{3}},
		},
		{
			"returning target alias",
			Update().TableAlias("t", "u").Set(Set("v", 1)).Where(Eq("v", 10)).
				Returning("u.*").SetDialect(dialect.Postgres),
			[][]any{{1}},
		},
	}
	type query struct {
		Name, SQL string
		Args      []any
		Want      [][]any
	}

	queries := make([]query, 0, len(cases)+3)
	for _, tc := range cases {
		s, args, err := tc.b.Build()
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		queries = append(queries, query{tc.name, s, args, tc.want})
	}

	p := Coalesce(Ident("v"), Param(0))
	shared := Param(0)
	compiled := []struct {
		b    *SelectBuilder
		args []any
		want [][]any
	}{
		{
			Select("d.v").FromSource(ValuesSource("d", []string{"v"}, []any{Param(0)}, []any{Param(1)}).
				ColumnTypes("INTEGER")).Where(Gt("d.v", Param(2))).
				SetDialect(dialect.Postgres),
			[]any{2, 10, 2},
			[][]any{{10}},
		},
		{
			Select().SelectExpr(p, Count("*")).From("t").GroupByExpr(p).
				Having(Gt(p, Param(1))).OrderByExpr(p, Asc).SetDialect(dialect.Postgres),
			[]any{0, 0},
			[][]any{{2, 2}, {10, 1}},
		},
		{
			Select().SelectExprAlias(Coalesce(Expr("NULL::INTEGER"), shared), "n").
				SelectExprAlias(Coalesce(Expr("NULL::TEXT"), shared), "s").Distinct().
				OrderByAsc("n").SetDialect(dialect.Postgres),
			[]any{"2"},
			[][]any{{2, "2"}},
		},
	}
	for _, tc := range compiled {
		tmpl, err := tc.b.Compile()
		if err != nil {
			t.Fatal(err)
		}

		s, args, err := tmpl.Bind(tc.args...)
		if err != nil {
			t.Fatal(err)
		}

		queries = append(queries, query{"compiled query", s, args, tc.want})
	}

	input, err := json.Marshal(queries)
	if err != nil {
		t.Fatal(err)
	}

	script := `import fs from 'node:fs';
import { pathToFileURL } from 'node:url';
const { PGlite } = await import(pathToFileURL(process.argv[1]).href);
const db = new PGlite();
const stringify = value => JSON.stringify(value, (_, v) => typeof v === 'bigint' ? Number(v) : v);
try {
  for (const q of JSON.parse(fs.readFileSync(0, 'utf8'))) {
    await db.exec('DROP TABLE IF EXISTS t, other; CREATE TABLE t(v INTEGER); INSERT INTO t VALUES (NULL), (2), (2), (10); CREATE TABLE other(v INTEGER); INSERT INTO other VALUES (1), (2);');
    try {
      const result = await db.query(q.SQL, q.Args ?? [], {rowMode: 'array'});
      if (stringify(result.rows) !== stringify(q.Want)) throw new Error('rows '+stringify(result.rows)+' expected '+stringify(q.Want));
    } catch (e) { throw new Error(q.Name+': '+q.SQL+'\n'+e.message, {cause: e}); }
  }
  console.log((await db.query('SELECT version()')).rows[0].version);
  console.log('PostgreSQL execution cases passed');
} finally { await db.close(); }
`
	cmd := exec.Command(node, "--input-type=module", "-e", script, module)
	cmd.Stdin = strings.NewReader(string(input))
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s\n%v", output, err)
	}
	t.Logf("%s%d execution cases passed", output, len(queries))
}
