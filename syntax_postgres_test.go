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

	scalar := Coalesce(Subquery(Select().SelectExpr(Sum("v")).From("other")), 0)
	cases := []struct {
		name string
		b    SQLBuilder
		want [][]any
	}{
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
