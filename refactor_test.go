package sqlx

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"reflect"
	"strings"
	"testing"

	"github.com/xgfone/go-op"
	"github.com/xgfone/go-sqlx/dialect"
)

func checkSQL(t *testing.T, b Statement, want string, args ...any) {
	t.Helper()

	q, a, e := b.Build()
	if e != nil || q != want || !reflect.DeepEqual(a, args) {
		t.Fatalf("got %q %#v %v; want %q %#v", q, a, e, want, args)
	}
}

func checkBuildError(t *testing.T, b Statement) {
	t.Helper()

	q, a, e := b.Build()
	if e == nil || q != "" || a != nil {
		t.Fatalf("expected build error: %q %v %v", q, a, e)
	}
}

func TestExplicitSortingHavingAndPagination(t *testing.T) {
	checkSQL(t, Select("id").From("t").OrderByDesc("time"),
		"SELECT `id` FROM `t` ORDER BY `time` DESC")
	checkSQL(t, Select().SelectExpr(Count("*")).From("t").Having(Expr("COUNT(*) > ?", 2).Condition()),
		"SELECT COUNT(*) FROM `t` HAVING (COUNT(*) > ?)", 2)
	checkSQL(t, Select("a.id").FromAlias("t", "a").FromAlias("t", "b"),
		"SELECT `a`.`id` FROM `t` AS `a`, `t` AS `b`")

	fixture := &scanFixture{}
	db := fixtureDB(t, fixture)
	b := db.Select("value").From("t").Pagination(op.PageSize(3, 10))
	r := b.QueryRowContext(context.Background())
	_ = r.Close()

	if fixture.query != `SELECT "value" FROM "t" LIMIT 1 OFFSET 20` {
		t.Fatal(fixture.query)
	}

	checkSQL(t, b, `SELECT "value" FROM "t" LIMIT 10 OFFSET 20`)
	checkSQL(t, b.Pagination(op.PageSize(2, 5)).Offset(7), `SELECT "value" FROM "t" LIMIT 5 OFFSET 7`)
}

func TestNestedBindingsAndJoinConditions(t *testing.T) {
	pg := &DB{Dialect: dialect.Postgres}
	sub := Select("id").From("u").Where(op.Eq("active", true))
	b := pg.Select().SelectExpr(Expr("COALESCE(?, ?)", Ident("a", "name"), "unknown")).
		FromSelect(sub, "a").
		JoinLeft("v", "b", op.Or(On("a.id", "b.id"), op.Gt("b.score", 10))).
		Where(Exists(Select().SelectExpr(Expr("1")).From("w").Where(op.Eq("kind", "x")))).
		Having(Expr("COUNT(*) > ?", 2).Condition())

	checkSQL(t, b,
		`SELECT COALESCE("a"."name", $1) FROM (SELECT "id" FROM "u" WHERE "active"=$2) AS "a" LEFT JOIN "v" AS "b" ON ("a"."id"="b"."id" OR "b"."score">$3) WHERE (EXISTS (SELECT 1 FROM "w" WHERE "kind"=$4)) HAVING (COUNT(*) > $5)`,
		"unknown", true, 10, "x", 2)

	checkSQL(t, pg.Select("id").From("t").Where(InQuery("id", sub)),
		`SELECT "id" FROM "t" WHERE "id" IN (SELECT "id" FROM "u" WHERE "active"=$1)`,
		true)

	checkSQL(t, pg.Update().Table("t").SetExpr("n", Expr("? + ?", Ident("n"), 3)).Where(op.Eq("id", 4)),
		`UPDATE "t" SET "n"="n" + $1 WHERE "id"=$2`,
		3, 4)
}

func TestExpressionTokenization(t *testing.T) {
	db := &DB{Dialect: dialect.Postgres}
	checkSQL(t, db.Select().SelectExpr(Expr("COALESCE('?', ?) /* ? */ -- ?\n", 7)),
		"SELECT COALESCE('?', $1) /* ? */ -- ?\n", 7)
	checkSQL(t, db.Select().SelectExpr(Expr("? ?? ? || $$?$$ || $tag$?$tag$", Ident("doc"), "key")),
		`SELECT "doc" ? $1 || $$?$$ || $tag$?$tag$`, "key")

	checkBuildError(t, db.Select().SelectExpr(Expr("? + ?", 1)))
	checkBuildError(t, db.Select().SelectExpr(Expr("'unterminated ?", 1)))
	checkBuildError(t, db.Select().SelectExpr(Expr("1", 1)))
}

func TestLockingAndDialectErrors(t *testing.T) {
	for _, d := range []Dialect{dialect.Postgres, dialect.MySQL} {
		db := &DB{Dialect: d}
		q, a, e := db.Select("id").FromAlias("t", "a").Where(op.Eq("id", 4)).ForUpdate("a").SkipLocked().Build()
		if e != nil || !strings.Contains(q, " FOR UPDATE OF ") || !strings.HasSuffix(q, " SKIP LOCKED") || len(a) != 1 {
			t.Fatalf("%s %v %v", q, a, e)
		}
	}

	checkSQL(t, Select("id").From("t").ForShare().NoWait(), "SELECT `id` FROM `t` FOR SHARE NOWAIT")
	checkBuildError(t, Select("id").From("t").ForUpdate().SetDB(&DB{Dialect: dialect.SQLite}))
	checkBuildError(t, Select("id").From("t").SkipLocked())
	checkBuildError(t, Select("id").From("t").Distinct().ForUpdate())
	checkSQL(t, Select("id").From("t").ForUpdate().SkipLocked().ClearLock(), "SELECT `id` FROM `t`")
}

func TestInsertAlignmentDefaultsAndErrors(t *testing.T) {
	checkSQL(t, Insert().Into("t").Row(ColValue("a", 1), ColValue("b", 2)).Row(ColValue("b", 3), ColValue("a", 4)),
		"INSERT INTO `t` (`a`, `b`) VALUES (?, ?), (?, ?)", 1, 2, 4, 3)
	checkBuildError(t, Insert().Into("t").Row(ColValue("a", 1)).Row(ColValue("b", 2)))
	checkBuildError(t, Insert().Into("t").Row(ColValue("a", 1), ColValue("a", 2)))
	checkBuildError(t, Insert().Into("t").Columns("id"))

	if r, e := Insert().Into("t").Columns("id").ExecContext(context.Background()); e == nil || r != nil {
		t.Fatalf("empty insert: %v %v", r, e)
	}

	checkSQL(t, Insert().Into("t").DefaultValues(), "INSERT INTO `t` () VALUES ()")
	checkSQL(t, Insert().Into("t").DefaultValues().SetDB(&DB{Dialect: dialect.Postgres}), `INSERT INTO "t" DEFAULT VALUES`)
	checkSQL(t, Insert().Into("t").Values(nil, Default()), "INSERT INTO `t` VALUES (?, DEFAULT)", nil)
	checkBuildError(t, Insert().Into("t").Values(Default()).SetDB(&DB{Dialect: dialect.SQLite}))
}

func TestReturningAndConflictPolicies(t *testing.T) {
	db := &DB{Dialect: dialect.Postgres}
	checkSQL(t, db.Insert().Into("t").Columns("id", "v").Values(1, "a").
		OnConflictDoUpdate([]string{"id"}, op.Set("v", Ident("excluded", "v"))).Returning("id"),
		`INSERT INTO "t" ("id", "v") VALUES ($1, $2) ON CONFLICT ("id") DO UPDATE SET "v"="excluded"."v" RETURNING "id"`,
		1, "a")

	checkSQL(t, db.Insert().Into("t").Values(1).OnConflictDoNothing(),
		`INSERT INTO "t" VALUES ($1) ON CONFLICT DO NOTHING`, 1)
	checkSQL(t, Insert().Into("t").Values(1).OnDuplicateKeyUpdate(op.Set("v", 2)),
		"INSERT INTO `t` VALUES (?) ON DUPLICATE KEY UPDATE `v`=?", 1, 2)

	checkBuildError(t, Insert().Into("t").Values(1).OnConflictDoNothing())
	checkBuildError(t, Insert().Into("t").Values(1).Returning("id"))
	checkSQL(t, db.Delete().From("t").Using("u", "a").Where(On("t.id", "a.id")).Returning("t.id"),
		`DELETE FROM "t" USING "u" AS "a" WHERE "t"."id"="a"."id" RETURNING "t"."id"`)

	f := &scanFixture{values: []driver.Value{"saved"}}
	sqlite := fixtureDB(t, f)
	var result struct {
		Value string `sql:"value"`
	}

	e := sqlite.Insert().Into("t").Values("saved").Returning("value").QueryRowContext(context.Background()).Scan(&result)
	if e != nil || result.Value != "saved" {
		t.Fatalf("%+v %v", result, e)
	}
	if !strings.HasSuffix(f.query, `RETURNING "value"`) {
		t.Fatal(f.query)
	}
	if _, e := sqlite.Insert().Into("t").Values(1).Returning("id").ExecContext(context.Background()); e == nil {
		t.Fatal("Exec discarded RETURNING")
	}
}

func TestCloneAppendAndReset(t *testing.T) {
	base := Select("id").From("t").GroupBy("id").GroupBy("v").Where(op.Eq("id", 1))
	derived := base.Clone().Select("v").Where(op.Eq("v", 2))

	checkSQL(t, base, "SELECT `id` FROM `t` WHERE `id`=? GROUP BY `id`, `v`", 1)
	checkSQL(t, derived, "SELECT `id`, `v` FROM `t` WHERE (`id`=? AND `v`=?) GROUP BY `id`, `v`", 1, 2)
	checkSQL(t, derived.ClearWhere().ClearGroupBy().ClearSelect().Select("v"), "SELECT `v` FROM `t`")

	db := &DB{Dialect: dialect.Postgres}
	b := db.Select("id").Limit(-1)
	checkBuildError(t, b)
	checkSQL(t, b.Reset().SelectExpr(Expr("1")), "SELECT 1")

	sub := Select("id").From("t").Where(op.Eq("id", 1))
	q := Select().SelectExpr(Subquery(sub))
	sub.Where(op.Eq("id", 2))
	checkSQL(t, q, "SELECT (SELECT `id` FROM `t` WHERE `id`=?)", 1)

	input := []any{1}
	insert := Insert().Into("t").Values(input...)
	input[0] = 2
	other := insert.Clone().Values(3)
	checkSQL(t, insert, "INSERT INTO `t` VALUES (?)", 1)
	checkSQL(t, other, "INSERT INTO `t` VALUES (?), (?)", 1, 3)
}

func TestTableOperAndSoftDeleteScopes(t *testing.T) {
	type Model struct {
		Value string `sql:"value"`
	}

	o := NewOper[Model]("t")
	db := &DB{Dialect: dialect.Postgres}
	o.SetDB(db)
	if o.GetDB() != db {
		t.Fatal("SetDB")
	}

	checkSQL(t, o.SelectStruct(), `SELECT "value" FROM "t"`)
	checkSQL(t, o.Active().Select("value"), `SELECT "value" FROM "t" WHERE "deleted_at" IS NULL`)

	custom := o.WithSoftCondition(op.Eq("deleted", false)).WithDeletedCondition(op.Eq("deleted", true))
	checkSQL(t, custom.Active().Select("value"), `SELECT "value" FROM "t" WHERE "deleted"=$1`, false)
	checkSQL(t, custom.Deleted().Select("value"), `SELECT "value" FROM "t" WHERE "deleted"=$1`, true)
	checkSQL(t, o.Select("value"), `SELECT "value" FROM "t"`)
	checkSQL(t, o.Table.Update().Set(op.Set("value", nil)), `UPDATE "t" SET "value"=$1`, nil)
}

func TestStructFieldConsistencyAndPointers(t *testing.T) {
	type Child struct {
		Count int `sql:"count"`
	}

	type Model struct {
		ID       int    `sql:"id,omitempty"`
		Child    *Child `sql:"child"`
		Optional *int   `sql:"optional"`
		hidden   string //nolint:unused
	}

	m := Model{ID: 2}
	checkSQL(t, Select().SelectStruct(m, "a").FromAlias("t", "a"),
		"SELECT `a`.`id`, `a`.`child_count`, `a`.`optional` FROM `t` AS `a`")
	checkSQL(t, Insert().Into("t").Struct(m),
		"INSERT INTO `t` (`id`, `child_count`, `optional`) VALUES (?, ?, ?)",
		2, nil, nil)

	e := ScanColumnsToStruct(func(vs ...any) error {
		*vs[0].(*int) = 5
		*vs[1].(**int) = nil
		return nil
	}, []string{"child_count", "optional"}, &m)
	if e != nil || m.Child == nil || m.Child.Count != 5 {
		t.Fatalf("%+v %v", m, e)
	}

	type Omit struct {
		A int `sql:"a,omitempty"`
		B int `sql:"b,omitempty"`
	}

	checkBuildError(t, Insert().Into("t").Structs([]Omit{{A: 1}, {B: 2}}))
	checkSQL(t, Insert().Into("t").Columns("b", "a").Structs([]*Omit{{A: 1}, {B: 2}}),
		"INSERT INTO `t` (`b`, `a`) VALUES (?, ?), (?, ?)", 0, 1, 2, 0)

	type Duplicate struct {
		A int `sql:"x"`
		B int `sql:"x"`
	}

	checkBuildError(t, Select().SelectStruct(Duplicate{}))
	checkBuildError(t, Insert().Into("t").Struct((*Model)(nil)))

	if e := ScanColumnsToStruct(func(...any) error { return nil }, []string{"id"}, (*Model)(nil)); e == nil {
		t.Fatal("nil destination accepted")
	}
	if e := ScanColumnsToStruct(func(...any) error { return nil }, []string{"id", "id"}, &m); e == nil {
		t.Fatal("ambiguous columns accepted")
	}

	type Recursive struct{ Next *Recursive }
	checkBuildError(t, Select().SelectStruct(Recursive{}))
}

func TestWildcardAndPointerRowsUseDriverMetadata(t *testing.T) {
	f := &scanFixture{values: []driver.Value{"first", "second"}}
	db := fixtureDB(t, f)
	var rows []*struct {
		Value string `sql:"value"`
	}

	if e := db.Select("*").From("t").QueryRowsContext(context.Background()).Bind(&rows); e != nil {
		t.Fatal(e)
	}

	if len(rows) != 2 || rows[0].Value != "first" || rows[1].Value != "second" {
		t.Fatalf("%+v", rows)
	}

	if e := db.Select("*").From("t").QueryRowsContext(context.Background()).Bind(nil); e == nil {
		t.Fatal("nil bind accepted")
	}
}

// These fixture transactions exercise database/sql's actual transaction dispatch.
type txConnector struct{ f *txFixture }
type txDriver struct{}
type txFixture struct {
	begun     int
	execs     int
	rolled    int
	committed int
	query     string
}

func (txDriver) Open(string) (driver.Conn, error)                  { return nil, errors.New("connector required") }
func (c txConnector) Driver() driver.Driver                        { return txDriver{} }
func (c txConnector) Connect(context.Context) (driver.Conn, error) { return &txConn{c.f}, nil }

type txConn struct{ f *txFixture }

func (c *txConn) Prepare(string) (driver.Stmt, error) { return nil, errors.New("unexpected prepare") }
func (c *txConn) Close() error                        { return nil }
func (c *txConn) Begin() (driver.Tx, error)           { c.f.begun++; return &txState{c.f}, nil }

func (c *txConn) ExecContext(_ context.Context, q string, _ []driver.NamedValue) (driver.Result, error) {
	c.f.execs++
	c.f.query = q
	return driver.RowsAffected(1), nil
}

type txState struct{ f *txFixture }

func (t *txState) Commit() error   { t.f.committed++; return nil }
func (t *txState) Rollback() error { t.f.rolled++; return nil }

func TestTransactionExecutor(t *testing.T) {
	f := &txFixture{}
	std := sql.OpenDB(txConnector{f})
	defer std.Close() //nolint:errcheck

	db := &DB{Dialect: dialect.Postgres, Executor: std}
	tx, e := db.BeginTx(context.Background(), nil)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback() //nolint:errcheck

	_, e = db.Update().Table("t").Set(op.Set("value", 7)).SetExecutor(tx).ExecContext(context.Background())
	if e != nil {
		t.Fatal(e)
	}

	_, e = db.WithExecutor(tx).Delete().From("t").Where(op.Eq("id", 1)).ExecContext(context.Background())
	if e != nil {
		t.Fatal(e)
	}

	if e = tx.Commit(); e != nil {
		t.Fatal(e)
	}

	if f.begun != 1 || f.committed != 1 || f.execs != 2 {
		t.Fatalf("%+v", f)
	}

	_, e = db.WithExecutor(tx).Insert().Into("t").Values(1).ExecContext(context.Background())
	if !errors.Is(e, sql.ErrTxDone) {
		t.Fatal(e)
	}
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
		b    Statement
		want [][]any
	}{
		{db.Insert().Into("t").Columns("id", "v", "n").Values(1, "one", 10).Returning("id"), [][]any{{1}}},
		{db.Insert().Into("t").Columns("id", "v", "n").Values(1, "two", 20).
			OnConflictDoUpdate(
				[]string{"id"},
				op.Set("v", Ident("excluded", "v")),
				op.Set("n", Ident("excluded", "n")),
			).
			Returning("v", "n"), [][]any{{"two", 20}}},
		{db.Update().Table("t").SetExpr("n", Expr("? + ?", Ident("n"), 2)).
			Where(op.Eq("id", 1)).Returning("n"), [][]any{{22}}},
		{db.Select().SelectExpr(Sum("n")).From("t").Having(Expr("SUM(n) > ?", 20).Condition()), [][]any{{22}}},
		{db.Select("id").With("filtered", Select("id").From("t").Where(op.Eq("n", 22))).
			From("filtered").UnionAll(Select().SelectExpr(Value(9))), [][]any{{1}, {9}}},
		{db.Insert().Into("archive").Columns("id").FromSelect(Select("id").From("t").
			Where(op.Eq("id", 1))).Returning("id"), [][]any{{1}}},
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

func ExampleSelectBuilder_ForUpdate() {
	db := &DB{Dialect: dialect.Postgres}
	q, args, err := db.Select("id", "balance").From("accounts").Where(op.Eq("id", 42)).ForUpdate().NoWait().Build()

	fmt.Println(q)
	fmt.Println(args, err)
	// Output:
	// SELECT "id", "balance" FROM "accounts" WHERE "id"=$1 FOR UPDATE NOWAIT
	// [42] <nil>
}

func TestExpressionPredicateGrouping(t *testing.T) {
	checkSQL(t, Select("id").From("t").Where(Expr("a=? OR b=?", 1, 2).Condition(), op.Eq("tenant", 3)),
		"SELECT `id` FROM `t` WHERE ((a=? OR b=?) AND `tenant`=?)", 1, 2, 3)
	checkSQL(t, Delete().From("t").Where(op.Eq("deleted_at", nil)), "DELETE FROM `t` WHERE `deleted_at` IS NULL")
	checkBuildError(t, Update().Table("t").Set(op.Set("v", 1)).Where(op.Gt("id", nil)))
}

type dynamicColumns struct{ column string }

func (v dynamicColumns) Columns(q string) []Namer { return []Namer{{Name: v.column}} }

type pointerValue struct{ Number int }

func (v *pointerValue) Value() (driver.Value, error) { return int64(v.Number), nil }

func TestStructProvidersAndPointerValuers(t *testing.T) {
	type Model struct {
		V pointerValue `sql:"v"`
	}

	checkSQL(t, Select().SelectStruct(dynamicColumns{"first"}).From("t"), "SELECT `first` FROM `t`")
	checkSQL(t, Select().SelectStruct(dynamicColumns{"second"}).From("t"), "SELECT `second` FROM `t`")
	checkSQL(t, Select().SelectStruct(Model{}).From("t"), "SELECT `v` FROM `t`")

	q, args, e := Insert().Into("t").Struct(Model{V: pointerValue{8}}).Build()
	if e != nil || q != "INSERT INTO `t` (`v`) VALUES (?)" {
		t.Fatalf("%s %v", q, e)
	}

	v, e := args[0].(driver.Valuer).Value()
	if e != nil || v != int64(8) {
		t.Fatalf("%v %v", v, e)
	}
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
