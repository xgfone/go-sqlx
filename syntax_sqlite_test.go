// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"encoding/json"
	"os/exec"
	"strings"
	"testing"

	"github.com/xgfone/go-sqlx/dialect"
)

func TestExtendedSyntaxSQLiteExecution(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 unavailable")
	}

	db := &DB{Dialect: dialect.SQLite}
	seq := Select().SelectExpr(Value(1)).UnionAll(Select().SelectExpr(Expr("? + 1", Ident("n"))).From("seq").Where(Lt("n", 4)))
	row := Tuple(Ident("id"), Ident("team"))
	window := Window().PartitionBy("team").OrderBy("id", Asc)
	cases := []struct {
		name string
		b    Statement
		want [][]any
	}{
		{
			"bracket identifier with marker",
			db.Select().SelectExpr(Expr("[why?] + ?", 2)).
				FromSelect(Select().SelectExprAlias(Value(40), "why?"), "q"),
			[][]any{{42}},
		},
		{
			"values source",
			db.Select("v.id", "v.label").
				FromSource(ValuesSource("v", []string{"id", "label"}, []any{1, "a"}, []any{2, "b"})).
				OrderByAsc("v.id"),
			[][]any{{1, "a"}, {2, "b"}},
		},
		{
			"case",
			db.Select("id").SelectExpr(Case().
				When(Ge("v", 20), "large").Else("small").End()).From("t").
				OrderByAsc("id"),
			[][]any{{1, "small"}, {2, "large"}, {3, "large"}},
		},
		{
			"simple case",
			db.Select().SelectExpr(CaseValue(Value(2)).
				WhenValue(1, "one").WhenValue(2, "two").End()),
			[][]any{{"two"}},
		},
		{
			"cast coalesce nullif",
			db.Select().SelectExpr(Cast(Value("12"), "INTEGER"), Coalesce(nil, NullIf(1, 1), 9)),
			[][]any{{12, 9}},
		},
		{
			"like escape",
			db.Select().SelectExpr(Value(1)).Where(Like(Value("a_b"), `a\_b`, `\`)),
			[][]any{{1}},
		},
		{
			"not between",
			db.Select("id").From("t").Where(NotBetween("v", 15, 25)).OrderByAsc("id"),
			[][]any{{1}, {3}},
		},
		{
			"in tuples",
			db.Select("id").From("t").Where(In(row, Tuple(1, "a"), Tuple(3, "b"))).OrderByAsc("id"),
			[][]any{{1}, {3}},
		},
		{
			"tuple greater",
			db.Select("id").From("t").Where(Gt(Tuple(Ident("v"), Ident("id")), Tuple(15, 0))).
				OrderByAsc("id"),
			[][]any{{2}, {3}},
		},
		{
			"empty in",
			db.Select("id").From("t").Where(In("id")),
			nil,
		},
		{
			"empty not in",
			db.Select().SelectExpr(Value(1)).Where(NotIn("id")),
			[][]any{{1}},
		},
		{
			"not in null",
			db.Select("id").From("t").Where(NotIn("id", 1, nil)),
			nil,
		},
		{
			"null safe helpers",
			db.Select().SelectExpr(Value(1)).Where(Eq(Value(nil), nil), Ne(Value(1), nil)),
			[][]any{{1}},
		},
		{
			"rank",
			db.Select("id").SelectExpr(RowNumber().Over(window)).From("t").OrderByAsc("id"),
			[][]any{{1, 1}, {2, 2}, {3, 1}},
		},
		{
			"filter window",
			db.Select("id").SelectExpr(Sum("v").Filter(Gt("v", 10)).Over(Window().
				PartitionBy("team"))).From("t").OrderByAsc("id"),
			[][]any{{1, 20}, {2, 20}, {3, 30}},
		},
		{
			"frame exclusion",
			db.Select("id").SelectExpr(Sum("v").
				Over(window.Rows(UnboundedPreceding(), CurrentRow()).Exclude(ExcludeCurrentRow))).
				From("t").OrderByAsc("id"),
			[][]any{{1, nil}, {2, 10}, {3, nil}},
		},
		{
			"range",
			db.Select("id").SelectExpr(Sum("v").Over(Window().OrderBy("v", Asc).
				Range(Preceding(10), CurrentRow()))).From("t").OrderByAsc("id"),
			[][]any{{1, 10}, {2, 30}, {3, 50}},
		},
		{
			"groups",
			db.Select("id").SelectExpr(Sum("v").Over(Window().OrderBy("team", Asc).
				Groups(CurrentRow(), CurrentRow()))).From("t").OrderByAsc("id"),
			[][]any{{1, 30}, {2, 30}, {3, 30}},
		},
		{
			"lag lead",
			db.Select("id").SelectExpr(
				Lag(Ident("v"), 1, 0).Over(window),
				Lead(Ident("v"), 1, 0).Over(window),
			).From("t").OrderByAsc("id"),
			[][]any{{1, 0, 20}, {2, 10, 0}, {3, 0, 0}},
		},
		{
			"named inherited window",
			db.Select("id").SelectExpr(Sum("v").OverName("running")).From("t").
				Window("by_team", Window().PartitionBy("team")).
				Window(
					"running",
					Window().BasedOn("by_team").OrderBy("id", Asc).
						Rows(UnboundedPreceding(), CurrentRow()),
				).OrderByAsc("id"),
			[][]any{{1, 10}, {2, 30}, {3, 30}},
		},
		{
			"null order",
			db.Select("q.x").FromSource(ValuesSource("q", []string{"x"}, []any{nil}, []any{2}, []any{1})).
				Sort(SortColumn{Column: "q.x", Order: Desc, Nulls: NullsLast}),
			[][]any{{2}, {1}, {nil}},
		},
		{
			"set precedence",
			db.Select().SelectExpr(Value(1)).Union(Select().SelectExpr(Value(2))).
				Intersect(Select().SelectExpr(Value(2))),
			[][]any{{2}},
		},
		{
			"mixed sets with CTE",
			db.Select("id").WithColumns("q", Select().SelectExpr(Value(1)), "id").From("q").
				Union(Select().SelectExpr(Value(2))).Intersect(Select("id").From("q")).
				Except(Select().SelectExpr(Value(3))),
			[][]any{{1}},
		},
		{
			"set right grouping",
			db.Select().SelectExpr(Value(1)).Except(Select().SelectExpr(Value(1)).
				Except(Select().SelectExpr(Value(1)))),
			[][]any{{1}},
		},
		{
			"operand pagination",
			db.Select("id").From("t").Where(Eq("id", 1)).UnionAll(Select("id").
				From("t").OrderByDesc("id").Limit(1)).OrderByAsc("id"),
			[][]any{{1}, {3}},
		},
		{
			"recursive cte",
			db.Select("n").WithCTE(NewCTE("seq", seq, "n").Recursive()).
				From("seq").OrderByAsc("n"),
			[][]any{{1}, {2}, {3}, {4}},
		},
		{
			"cte update",
			db.Update().With("chosen", Select("id").From("t").Where(Eq("id", 2))).
				Table("t").Set(Set("v", 99)).Where(InQuery("id", Select("id").From("chosen"))).
				Returning("v"),
			[][]any{{99}},
		},
		{
			"cte delete",
			db.Delete().With("chosen", Select("id").From("t").Where(Eq("id", 2))).
				From("t").Where(InQuery("id", Select("id").From("chosen"))).
				Returning("id"),
			[][]any{{2}},
		},
		{
			"cte insert",
			db.Insert().Into("t").Columns("id", "team", "v").
				WithCTE(NewCTE("q", Select().SelectExpr(Value(4), Value("c"), Value(40)), "id", "team", "v")).
				FromSelect(Select("*").From("q")).Returning("id", "v"),
			[][]any{{4, 40}},
		},
		{
			"update values source",
			db.Update().Table("t").FromSource(ValuesSource("q", []string{"id", "v"}, []any{2, 200})).
				Set(Set("v", Ident("q", "v"))).Where(On("t.id", "q.id")).Returning("id", "v"),
			[][]any{{2, 200}},
		},
		{
			"row assignment",
			db.Update().Table("t").Set(SetRow([]string{"team", "v"}, "changed", 77)).
				Where(Eq("id", 1)).Returning("team", "v"),
			[][]any{{"changed", 77}},
		},
		{
			"left derived join",
			db.Select("t.id", "q.v").From("t").JoinLeftSelect(Select("id", "v").
				From("t").Where(Eq("id", 2)), "q", On("t.id", "q.id")).OrderByAsc("t.id"),
			[][]any{{1, nil}, {2, 20}, {3, nil}},
		},
		{
			"outer using",
			db.Select("id").From("t").JoinFullUsing("other", "", "id").OrderByAsc("id"),
			[][]any{{1}, {2}, {3}, {4}},
		},
		{
			"upsert select",
			db.Insert().Into("t").Columns("id", "team", "v").
				FromSelect(Select("id", "team", "v").From("t")).
				OnConflictDoNothing().Returning("id"),
			nil,
		},
		{
			"upsert compound select",
			db.Insert().Into("t").Columns("id", "team", "v").
				FromSelect(Select("id", "team", "v").From("t").Where(Gt("id", 0)).
					UnionAll(Select("id", "team", "v").From("t"))).
				OnConflictDoNothing().Returning("id"),
			nil,
		},
		{
			"targetless update",
			db.Insert().Into("t").Columns("id", "team", "v").Values(2, "a", 40).
				OnConflictDoUpdate(nil, Set("v", Excluded("v"))).Returning("id", "v"),
			[][]any{{2, 40}},
		},
		{
			"conditional update false",
			db.Insert().Into("t").Columns("id", "v").Values(2, 5).
				OnConflict(ConflictColumns("id").DoUpdate(Set("v", Excluded("v"))).
					Where(Gt(Excluded("v"), Ident("t", "v")))).Returning("id"),
			nil,
		},
		{
			"partial expression target",
			db.Insert().IntoAlias("users", "u").Columns("email", "v").
				Values("ONE", 20).OnConflict(ConflictExpressions(Func("lower", Ident("email"))).
				Where(IsNull("deleted_at")).DoUpdate(Set("v", Excluded("v"))).
				Where(Gt(Excluded("v"), Ident("u", "v")))).
				Returning("email", "v"),
			[][]any{{"one", 20}},
		},
		{
			"multiple conflicts",
			db.Insert().Into("users").Columns("id", "email", "v").Values(99, "one", 50).
				OnConflict(ConflictColumns("id").DoNothing(), ConflictColumns().
					DoUpdate(Set("v", Excluded("v")))).Returning("id", "v"),
			[][]any{{1, 50}},
		},
		{
			"literal backslash",
			db.Select().SelectExpr(Expr(`'\', ?`, 1)),
			[][]any{{`\`, 1}},
		},
	}

	type query struct {
		Name, SQL string
		Args      []any
		Want      [][]any
	}

	queries := make([]query, len(cases))
	for i, tc := range cases {
		q, args, e := tc.b.Build()
		if e != nil {
			t.Fatalf("%s: %v", tc.name, e)
		}
		queries[i] = query{tc.name, q, args, tc.want}
	}

	data, e := json.Marshal(queries)
	if e != nil {
		t.Fatal(e)
	}

	script := `import json,sqlite3,sys
if sqlite3.sqlite_version_info < (3,39):
    print('SKIP SQLite older than 3.39'); sys.exit(0)
schema="""
CREATE TABLE t(id INTEGER PRIMARY KEY,team TEXT,v INTEGER DEFAULT 7);
INSERT INTO t VALUES(1,'a',10),(2,'a',20),(3,'b',30);
CREATE TABLE other(id INTEGER PRIMARY KEY); INSERT INTO other VALUES(2),(4);
CREATE TABLE users(id INTEGER PRIMARY KEY,email TEXT,v INTEGER,deleted_at TEXT);
CREATE UNIQUE INDEX active_email ON users(lower(email)) WHERE deleted_at IS NULL;
INSERT INTO users VALUES(1,'one',10,NULL);
"""
cases=json.load(sys.stdin)
for q in cases:
    db=sqlite3.connect(':memory:'); db.executescript(schema)
    try:
        got=[list(row) for row in db.execute(q['SQL'],q['Args'] or [])]
        assert got==(q['Want'] or []), (q['Name'],q['SQL'],got,q['Want'])
    except Exception as e:
        raise AssertionError((q['Name'],q['SQL'],q['Args'],str(e))) from e
    finally:
        db.close()
print('SQLite',sqlite3.sqlite_version,':',len(cases),'execution cases passed')
`
	cmd := exec.Command(python, "-c", script)
	cmd.Stdin = strings.NewReader(string(data))
	output, e := cmd.CombinedOutput()
	if e != nil {
		t.Fatalf("%s\n%v", output, e)
	}
	if strings.HasPrefix(string(output), "SKIP") {
		t.Skip(string(output))
	}
	t.Log(string(output))
}
