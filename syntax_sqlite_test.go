// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"encoding/json"
	"fmt"
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
	numericValue := Numeric(Column("v").Typed[int64]())
	typedID := NewTypedColumn[int64]("id")
	typedValue := NewTypedColumn[int64]("v")
	seq := Select().SelectExpr(Value(1)).UnionAll(Select().SelectExpr(Expr("? + 1", Ident("n"))).From("seq").Where(Lt("n", 4)))
	row := Tuple(Ident("id"), Ident("team"))
	window := Window().PartitionBy("team").OrderBy("id", Asc)
	largeValues := make([][]any, 501)
	for i := range largeValues {
		largeValues[i] = []any{i, i + 1}
	}

	wideColumns := make([]string, 32)
	wideValues := make([][]any, 3)
	for i := range wideColumns {
		wideColumns[i] = fmt.Sprintf("column%d", 32-i)
	}
	wideColumns[8] = `a"quote`
	for i := range wideValues {
		wideValues[i] = make([]any, len(wideColumns))
		for j := range wideValues[i] {
			wideValues[i][j] = i*32 + j
		}
	}
	wideValues[0][0], wideValues[2][0] = nil, "text"

	cases := []struct {
		name string
		b    SQLBuilder
		want [][]any
	}{
		{
			"returning target name with alias",
			db.Update().TableAlias("main.t", "u").Set(Set("v", 99)).Where(Eq("u.id", 1)).Returning("t.v"),
			[][]any{{99}},
		},
		{
			"typed ordered comparisons",
			db.Select("id").From("t").Where(typedID.Gt(0), typedID.Ge(1), typedID.Lt(3), typedID.Le(2)).OrderByAsc("id"),
			[][]any{{1}, {2}},
		},
		{
			"typed inclusive range",
			db.Select("id").From("t").Where(typedValue.Between(10, 20)).OrderByAsc("id"),
			[][]any{{1}, {2}},
		},
		{
			"typed negative inclusive range",
			db.Select("id").From("t").Where(typedValue.NotBetween(10, 20)).OrderByAsc("id"),
			[][]any{{3}},
		},
		{
			"typed lists and slices",
			db.Select("id").From("t").Where(typedID.In(1, 2), typedID.NotIn(3),
				typedID.InSlice([]int64{1, 2, 3}), typedID.NotInSlice([]int64{3})).OrderByAsc("id"),
			[][]any{{1}, {2}},
		},
		{
			"typed empty in list",
			db.Select("id").From("t").Where(typedID.In()),
			nil,
		},
		{
			"typed empty not in slice",
			db.Select("id").From("t").Where(typedID.NotInSlice(nil)).OrderByAsc("id"),
			[][]any{{1}, {2}, {3}},
		},
		{
			"typed null ordered comparison",
			db.Select("id").From("users").Where(NewTypedColumn[*string]("deleted_at").Gt(nil)),
			nil,
		},
		{
			"typed null in list",
			db.Select("id").From("t").Where(NewTypedColumn[any]("v").In(10, nil)),
			[][]any{{1}},
		},
		{
			"typed null not in list",
			db.Select("id").From("t").Where(NewTypedColumn[any]("v").NotIn(10, nil)),
			nil,
		},
		{
			"typed expression comparisons",
			db.Select("id").From("t").Where(typedValue.GtExpr(Expr("? - ?", 15, 5)), typedValue.GeExpr(Value(20)),
				typedValue.LtExpr(Expr("? + ?", 25, 5)), typedValue.LeExpr(Value(30))),
			[][]any{{2}},
		},
		{
			"typed expression ranges",
			db.Select("id").From("t").Where(typedValue.BetweenExpr(typedID.Ref(), Value(20)),
				typedValue.NotBetweenExpr(Value(11), Value(19))).OrderByAsc("id"),
			[][]any{{1}, {2}},
		},
		{
			"typed expression lists",
			db.Select("id").From("t").Where(typedValue.InExpr(Value(10), Expr("? * ?", 10, 3)),
				typedValue.NotInExpr(Value(20))).OrderByAsc("id"),
			[][]any{{1}, {3}},
		},
		{
			"typed in query",
			db.Select("id").From("t").Where(typedID.InQuery(db.Select("id").From("other"))),
			[][]any{{2}},
		},
		{
			"typed not in query",
			db.Select("id").From("t").Where(typedID.NotInQuery(db.Select("id").From("other"))).OrderByAsc("id"),
			[][]any{{1}, {3}},
		},
		{
			"typed data and expression assignment",
			db.Update().Table("t").Set(Column("v").Typed[int64]().Set(42),
				Column("team").Typed[string]().SetExpr(Func("UPPER", Ident("team")))).
				Where(Column("id").Typed[int64]().Eq(1)).Returning("v", "team"),
			[][]any{{42, "A"}},
		},
		{
			"typed column assignment",
			db.Update().Table("t").Set(Column("v").Typed[int64]().SetFrom(Column("id").Typed[int64]())).
				Where(Eq("id", 2)).Returning("v"),
			[][]any{{2}},
		},
		{
			"typed insert values",
			db.Insert().Into("t").Row(Column("id").Typed[int64]().ColValue(4),
				Column("team").Typed[string]().ColValue("c"),
				Column("v").Typed[int64]().ColValueExpr(Expr("? + ?", 20, 2))).Returning("id", "v"),
			[][]any{{4, 22}},
		},
		{
			"typed null assignment and comparison",
			db.Update().Table("users").Set(Column("deleted_at").Typed[*string]().Set(nil)).
				Where(Column("deleted_at").Typed[*string]().Eq(nil)).Returning("id", "deleted_at"),
			[][]any{{1, nil}},
		},
		{
			"numeric add data",
			db.Update().Table("t").Set(numericValue.Add(2)).Where(Eq("id", 1)).Returning("v"),
			[][]any{{12}},
		},
		{
			"numeric subtract negative data",
			db.Update().Table("t").Set(numericValue.Sub(-3)).Where(Eq("id", 1)).Returning("v"),
			[][]any{{13}},
		},
		{
			"numeric multiply data",
			db.Update().Table("t").Set(numericValue.Mul(2)).Where(Eq("id", 1)).Returning("v"),
			[][]any{{20}},
		},
		{
			"numeric divide data",
			db.Update().Table("t").Set(numericValue.Div(3)).Where(Eq("id", 1)).Returning("v"),
			[][]any{{3}},
		},
		{
			"numeric increment",
			db.Update().Table("t").Set(numericValue.Inc()).Where(Eq("id", 1)).Returning("v"),
			[][]any{{11}},
		},
		{
			"numeric decrement",
			db.Update().Table("t").Set(numericValue.Dec()).Where(Eq("id", 1)).Returning("v"),
			[][]any{{9}},
		},
		{
			"numeric add expression",
			db.Update().Table("t").Set(numericValue.AddExpr(Expr("? - ?", 5, 2))).Where(Eq("id", 1)).Returning("v"),
			[][]any{{13}},
		},
		{
			"numeric subtract expression precedence",
			db.Update().Table("t").Set(numericValue.SubExpr(Expr("? + ?", 2, 3))).Where(Eq("id", 1)).Returning("v"),
			[][]any{{5}},
		},
		{
			"numeric multiply expression precedence",
			db.Update().Table("t").Set(numericValue.MulExpr(Expr("? + ?", 2, 3))).Where(Eq("id", 1)).Returning("v"),
			[][]any{{50}},
		},
		{
			"numeric divide expression precedence",
			db.Update().Table("t").Set(numericValue.DivExpr(Expr("? * ?", 2, 5))).Where(Eq("id", 1)).Returning("v"),
			[][]any{{1}},
		},
		{
			"numeric explicit floating division",
			db.Update().Table("t").Set(numericValue.DivExpr(Value(1.6))).Where(Eq("id", 1)).Returning("v"),
			[][]any{{6.25}},
		},
		{
			"numeric division by zero follows sqlite",
			db.Update().Table("t").Set(numericValue.Div(0)).Where(Eq("id", 1)).Returning("v"),
			[][]any{{nil}},
		},
		{
			"numeric upsert",
			db.Insert().Into("t").Columns("id", "v").Values(1, 4).
				OnConflict(ConflictColumns("id").DoUpdate(numericValue.AddExpr(Excluded("v")))).Returning("v"),
			[][]any{{14}},
		},
		{
			"arithmetic add expression",
			db.Update().Table("t").Set(Column("v").Add(Expr("? - ?", 5, 2))).Where(Eq("id", 1)).Returning("v"),
			[][]any{{13}},
		},
		{
			"arithmetic subtract expression precedence",
			db.Update().Table("t").Set(Sub("v", Expr("? + ?", 2, 3))).Where(Eq("id", 1)).Returning("v"),
			[][]any{{5}},
		},
		{
			"arithmetic multiply expression precedence",
			db.Update().Table("t").Set(Mul("v", Expr("? + ?", 2, 3))).Where(Eq("id", 1)).Returning("v"),
			[][]any{{50}},
		},
		{
			"arithmetic divide expression precedence",
			db.Update().Table("t").Set(Column("v").Div(Expr("? * ?", 2, 5))).Where(Eq("id", 1)).Returning("v"),
			[][]any{{1}},
		},
		{
			"arithmetic integer division",
			db.Update().Table("t").Set(Div("v", 3)).Where(Eq("id", 1)).Returning("v"),
			[][]any{{3}},
		},
		{
			"arithmetic floating division",
			db.Update().Table("t").Set(Div("v", 1.6)).Where(Eq("id", 1)).Returning("v"),
			[][]any{{6.25}},
		},
		{
			"arithmetic division by zero follows sqlite",
			db.Update().Table("t").Set(Div("v", 0)).Where(Eq("id", 1)).Returning("v"),
			[][]any{{nil}},
		},
		{
			"arithmetic increment",
			db.Update().Table("t").Set(Inc("v")).Where(Eq("id", 1)).Returning("v"),
			[][]any{{11}},
		},
		{
			"arithmetic decrement",
			db.Update().Table("t").Set(Column("v").Dec()).Where(Eq("id", 1)).Returning("v"),
			[][]any{{9}},
		},
		{
			"arithmetic upsert",
			db.Insert().Into("t").Columns("id", "v").Values(1, 4).
				OnConflict(ConflictColumns("id").DoUpdate(Add("v", Excluded("v")))).Returning("v"),
			[][]any{{14}},
		},
		{
			"returning wildcard with alias",
			db.Delete().FromAlias("t", "u").Where(Eq("u.id", 1)).Returning("*"),
			[][]any{{1, "a", 10}},
		},
		{
			"returning scalar aggregate subquery",
			db.Insert().IntoAlias("t", "u").Columns("id", "team", "v").Values(4, "c", 40).
				ReturningExpr(Coalesce(Subquery(Select().SelectExpr(Sum("id")).From("other")), 0), "total"),
			[][]any{{6}},
		},
		{
			"typed values source",
			db.Select("d.v").FromSource(ValuesSource("d", []string{"v"}, []any{"2"}, []any{"10"}).ColumnTypes("INTEGER")).Where(Gt("d.v", 2)),
			[][]any{{10}},
		},
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
			"values source beyond compound select limit",
			db.Select("v.id", "v.next").
				FromSource(ValuesSource("v", []string{"id", "next"}, largeValues...)).
				OrderByAsc("v.id"),
			largeValues,
		},
		{
			"values source aliases resembling generated names",
			db.Select("v.column1", "v.column2").
				FromSource(ValuesSource("v", []string{"column2", "column1"}, []any{1, 2})),
			[][]any{{2, 1}},
		},
		{
			"values source wide columns and mixed values",
			db.Select("*").FromSource(ValuesSource("v", wideColumns, wideValues...)).OrderByAsc("v.column31"),
			wideValues,
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
			"groups without ordering",
			db.Select("id").SelectExpr(Sum("v").Over(Window().
				Groups(UnboundedPreceding(), CurrentRow()))).From("t").OrderByAsc("id"),
			[][]any{{1, 60}, {2, 60}, {3, 60}},
		},
		{
			"partitioned groups without ordering",
			db.Select("id").SelectExpr(Sum("v").Over(Window().PartitionBy("team").
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
			db.Select("id").With("q", Select().SelectExpr(Value(1)), "id").From("q").
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
				With("q", Select().SelectExpr(Value(4), Value("c"), Value(40)), "id", "team", "v").
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
				OnConflict(ConflictColumns().DoNothing()).Returning("id"),
			nil,
		},
		{
			"upsert compound select",
			db.Insert().Into("t").Columns("id", "team", "v").
				FromSelect(Select("id", "team", "v").From("t").Where(Gt("id", 0)).
					UnionAll(Select("id", "team", "v").From("t"))).
				OnConflict(ConflictColumns().DoNothing()).Returning("id"),
			nil,
		},
		{
			"targetless update",
			db.Insert().Into("t").Columns("id", "team", "v").Values(2, "a", 40).
				OnConflict(ConflictColumns().DoUpdate(Set("v", Excluded("v")))).Returning("id", "v"),
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
				OnConflict(ConflictColumns("id").DoNothing()).
				OnConflict(ConflictColumns().DoUpdate(Set("v", Excluded("v")))).Returning("id", "v"),
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
    if q['Name'].startswith('values source') and hasattr(db,'setlimit'):
        db.setlimit(sqlite3.SQLITE_LIMIT_COMPOUND_SELECT,2)
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

// TestSQLiteExecution validates generated statements and their bound arguments on
// a real SQLite engine without adding a driver dependency to the library.
func TestSQLiteExecution(t *testing.T) {
	python, e := exec.LookPath("python3")
	if e != nil {
		t.Skip("python3 with sqlite3 unavailable")
	}

	db := &DB{Dialect: dialect.SQLite}
	type query struct {
		SQL  string
		Args []any
		Want [][]any
	}

	cases := []struct {
		b    SQLBuilder
		want [][]any
	}{
		{db.Insert().Into("t").Columns("id", "v", "n").Values(1, "one", 10).Returning("id"), [][]any{{1}}},
		{db.Insert().Into("t").Columns("id", "v", "n").Values(1, "two", 20).
			OnConflict(ConflictColumns("id").DoUpdate(Set("v", Ident("excluded", "v")), Set("n", Ident("excluded", "n")))).
			Returning("v", "n"), [][]any{{"two", 20}}},
		{db.Update().Table("t").SetExpr("n", Expr("? + ?", Ident("n"), 2)).
			Where(Eq("id", 1)).Returning("n"), [][]any{{22}}},
		{db.Select().SelectExpr(Sum("n")).From("t").Having(Expr("SUM(n) > ?", 20).Condition()), [][]any{{22}}},
		{db.Select("id").With("filtered", Select("id").From("t").Where(Eq("n", 22))).
			From("filtered").UnionAll(Select().SelectExpr(Value(9))), [][]any{{1}, {9}}},
		{db.Insert().Into("archive").Columns("id").FromSelect(Select("id").From("t").
			Where(Eq("id", 1))).Returning("id"), [][]any{{1}}},
		{db.Select("a.id").FromAlias("t", "a").Join("archive", "b", On("a.id", "b.id")), [][]any{{1}}},
		{db.Delete().From("t").Where(InQuery("id", Select("id").From("archive"))).Returning("id"), [][]any{{1}}},
		{db.Insert().Into("t").DefaultValues().Returning("v"), [][]any{{"default"}}},
	}

	qs := make([]query, len(cases))
	for i, c := range cases {
		q, a, e := c.b.Build()
		if e != nil {
			t.Fatal(e)
		}
		qs[i] = query{q, a, c.want}
	}
	data, _ := json.Marshal(qs)

	script := `import sys,json,sqlite3
if sqlite3.sqlite_version_info < (3,39):
    print('SKIP old SQLite'); sys.exit(0)
c=sqlite3.connect(':memory:')
c.executescript("CREATE TABLE t(id INTEGER PRIMARY KEY,v TEXT DEFAULT 'default',n INTEGER); CREATE TABLE archive(id INTEGER);")
for q in json.load(sys.stdin):
    got=[list(row) for row in c.execute(q['SQL'],q['Args'] or [])]
    assert got==q['Want'], (q,got)
print('SQLite',sqlite3.sqlite_version,'passed')
`

	cmd := exec.Command(python, "-c", script)
	cmd.Stdin = strings.NewReader(string(data))
	out, e := cmd.CombinedOutput()
	if e != nil {
		t.Fatalf("SQLite: %s %v", out, e)
	}
	if strings.HasPrefix(string(out), "SKIP") {
		t.Skip(string(out))
	}
	t.Log(string(out))
}
