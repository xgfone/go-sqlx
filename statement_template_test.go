// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/xgfone/go-sqlx/dialect"
)

type templateBuilder interface {
	Compile() (*StatementTemplate, error)
	SQLBuilder
}

func mustCompileTemplate(t testing.TB, b templateBuilder) *StatementTemplate {
	t.Helper()
	q, err := b.Compile()
	if err != nil {
		t.Fatal(err)
	}
	return q
}

func TestTemplateMatchesBuild(t *testing.T) {
	type queryCase struct {
		name string
		make func(any, any) templateBuilder
	}

	for _, d := range []Dialect{dialect.MySQL, dialect.Postgres, dialect.SQLite} {
		cases := []queryCase{
			{"select", func(a, b any) templateBuilder {
				return Select("id").From("t").Where(Eq("a", b), In("b", a, 8, a)).
					Limit(100).SetDialect(d)
			}},
			{"expression", func(a, b any) templateBuilder {
				return Select().SelectExpr(Expr("COALESCE(?, ?) + '?' /* ? */", a, b)).
					SetDialect(d)
			}},
			{"nested_cte", func(a, b any) templateBuilder {
				inner := Select("id").From("src").Where(Eq("id", a))
				return Select("id").WithCTE(NewCTE("c", inner)).From("c").
					Where(Exists(Select("id").From("u").Where(Eq("id", b))), Eq("id", a)).
					SetDialect(d)
			}},
			{"insert", func(a, b any) templateBuilder {
				return Insert().Into("t").Columns("a", "b").Values(a, b).Values(b, a).
					SetDialect(d)
			}},
			{"insert_select", func(a, b any) templateBuilder {
				return Insert().Into("t").Columns("a").FromSelect(
					Select().SelectExpr(Expr("? + ?", a, b)),
				).SetDialect(d)
			}},
			{"update", func(a, b any) templateBuilder {
				return Update().Table("t").Set(Set("a", a), Set("b", Expr("? + 1", b))).
					Where(Eq("id", a)).SetDialect(d)
			}},
			{"delete", func(a, b any) templateBuilder {
				return Delete().From("t").Where(Between("id", a, b)).SetDialect(d)
			}},
			{"custom", func(a, b any) templateBuilder {
				return Select("id").Where(ConditionFunc(func(c *BuildContext) string {
					return c.Quote("id") + "=" + c.Add(a) + " OR " + c.Quote("id") + "=" + c.Value(b)
				})).SetDialect(d)
			}},
		}
		if d.Name() == "postgres" || d.Name() == "sqlite3" {
			cases = append(cases, queryCase{"upsert_returning", func(a, b any) templateBuilder {
				return Insert().Into("t").Columns("id", "v").Values(a, b).
					OnConflict(
						ConflictColumns("id").
							DoUpdate(Set("v", Expr("? + ?", Excluded("v"), b))).
							Where(Gt("id", a)),
					).Returning("id").SetDialect(d)
			}})
		} else {
			cases = append(cases, queryCase{"duplicate_update", func(a, b any) templateBuilder {
				return Insert().Into("t").Columns("id", "v").Values(a, b).
					OnDuplicateKeyUpdate(Set("v", b)).SetDialect(d)
			}})
		}

		for _, tc := range cases {
			t.Run(d.Name()+"/"+tc.name, func(t *testing.T) {
				wantSQL, wantArgs, err := tc.make(int64(42), int64(99)).Build()
				if err != nil {
					t.Fatal(err)
				}

				q := mustCompileTemplate(t, tc.make(Param(0), Param(1)))
				gotSQL, gotArgs, err := q.Bind(int64(42), int64(99))
				if err != nil || gotSQL != wantSQL || !reflect.DeepEqual(gotArgs, wantArgs) {
					t.Fatalf("got %q %v %v, want %q %v", gotSQL, gotArgs, err, wantSQL, wantArgs)
				}
			})
		}
	}
}

func TestTemplateNullAndNamedParameters(t *testing.T) {
	q := mustCompileTemplate(t, Select("id").Where(Eq("id", Param(0))).SetDialect(dialect.Postgres))
	s, args, err := q.Bind(nil)
	if err != nil || s != `SELECT "id" WHERE ("id" = $1)` || !reflect.DeepEqual(args, []any{nil}) {
		t.Fatal(s, args, err)
	}

	for _, d := range []Dialect{dialect.SQLite, dialect.MySQL, dialect.Postgres} {
		named := sql.Named("fixed", 9)
		makeQuery := func(v any) *SelectBuilder {
			return Select().SelectExpr(Expr("? + ? + ? + ?", named, v, named, v)).SetDialect(d)
		}

		q := mustCompileTemplate(t, makeQuery(Param(0)))
		wantSQL, wantArgs, _ := makeQuery(3).Build()
		gotSQL, gotArgs, err := q.Bind(3)
		if err != nil || gotSQL != wantSQL || !reflect.DeepEqual(gotArgs, wantArgs) {
			t.Fatal(gotSQL, gotArgs, err, wantSQL, wantArgs)
		}
	}
}

func TestTemplateDefaultAndLexicalRules(t *testing.T) {
	for _, d := range []Dialect{dialect.MySQL, dialect.Postgres} {
		makeQuery := func(value any) *InsertBuilder {
			return Insert().Into("t").Columns("id", "v").Values(value, Default()).
				Values(9, Expr("? + 1", value)).SetDialect(d)
		}

		q := mustCompileTemplate(t, makeQuery(Param(0)))
		wantSQL, wantArgs, _ := makeQuery(7).Build()
		gotSQL, gotArgs, err := q.Bind(7)
		if err != nil || gotSQL != wantSQL || !reflect.DeepEqual(gotArgs, wantArgs) {
			t.Fatal(gotSQL, gotArgs, err, wantSQL, wantArgs)
		}
	}

	for _, tc := range []struct {
		d   Dialect
		sql string
	}{
		{dialect.Postgres, "$tag$?$tag$ || ? /* outer /* ? */ */"},
		{dialect.Postgres, "? ?? 'key'"},
		{dialect.MySQL, "? # ignored ?\n"},
		{dialect.SQLite, "[why?] + ?"},
	} {
		makeQuery := func(value any) *SelectBuilder {
			return Select().SelectExpr(Expr(tc.sql, value)).SetDialect(tc.d)
		}

		q := mustCompileTemplate(t, makeQuery(Param(0)))
		wantSQL, wantArgs, _ := makeQuery("data").Build()
		gotSQL, gotArgs, err := q.Bind("data")
		if err != nil || gotSQL != wantSQL || !reflect.DeepEqual(gotArgs, wantArgs) {
			t.Fatal(gotSQL, gotArgs, err, wantSQL, wantArgs)
		}
	}

	// DEFAULT and RETURNING still obey the compile-time dialect capabilities.
	_, err := Insert().Into("t").Values(Param(0), Default()).SetDialect(dialect.SQLite).Compile()
	if err == nil {
		t.Fatal("unsupported DEFAULT accepted")
	}

	_, err = Insert().Into("t").Values(Param(0)).Returning("id").SetDialect(dialect.MySQL).Compile()
	if err == nil {
		t.Fatal("unsupported RETURNING accepted")
	}
}

func TestTemplateValidation(t *testing.T) {
	maxInt := int(^uint(0) >> 1)
	for _, b := range []templateBuilder{
		Select("id").Where(Eq("id", Param(-1))),
		Select("id").Where(Eq("id", Param(1))),
		Select("id").Where(In("id", Param(0), Param(2))),
		Select("id").Where(Eq("id", Param(maxInt))),
		Select().SelectExpr(Value(sql.Named("v", Param(0)))),
		Select().SelectExpr(Value(sql.Named("", Param(0)))),
		Select().SelectExpr(Expr("? ?", Param(0))),
		Select().SelectExpr(Expr("'unterminated ?", Param(0))),
		Insert().Into("t").Values(Param(0)).Values(1, 2),
		Select(),
		Update().Table("t"),
		Delete(),
		(*SelectBuilder)(nil),
		(*InsertBuilder)(nil),
		(*UpdateBuilder)(nil),
		(*DeleteBuilder)(nil),
	} {
		q, err := b.Compile()
		if err == nil || q != nil {
			t.Fatalf("invalid %T compiled: %v %v", b, q, err)
		}
	}

	for _, b := range []SQLBuilder{
		Select().SelectExpr(Param(0)),
		Select("id").Where(OnArg("id", Param(0))),
		Insert().Into("t").Values(Param(0)),
		Update().Table("t").Set(Set("id", Param(0))),
		Delete().From("t").Where(Eq("id", Param(0))),
	} {
		if _, _, err := b.Build(); err == nil || !strings.Contains(err.Error(), "Compile") {
			t.Fatalf("unbound Param accepted: %T %v", b, err)
		}
	}

	q := mustCompileTemplate(t, Select().SelectExpr(Param(0)))
	for _, params := range [][]any{
		nil,
		{1, 2},
		{Param(0)},
		{Ident("id")},
		{sql.Named("v", 1)},
		{Select("id")},
	} {
		if s, a, err := q.Bind(params...); err == nil || s != "" || a != nil {
			t.Fatalf("bad parameters accepted: %v", params)
		}
	}

	for _, q := range []*StatementTemplate{nil, {}} {
		if _, _, err := q.Bind(); err == nil {
			t.Fatal("uncompiled template accepted")
		}
		if err := q.QueryRowsContext(context.Background(), nil).Err(); err == nil {
			t.Fatal("uncompiled template queried")
		}
		if _, err := q.ExecContext(context.Background(), nil); err == nil {
			t.Fatal("uncompiled template executed")
		}
	}

	// A failed/compiled build must not leave compilation enabled in the pool.
	checkBuildError(t, Select().SelectExpr(Param(0)))
	checkSQL(t, Select().SelectExpr(Value(5)).SetDialect(dialect.Postgres), "SELECT $1", 5)
}

func TestTemplateSnapshotAndCustomRenderer(t *testing.T) {
	calls := 0
	fixed := []byte("abc")
	b := Select("id").From("t").Where(ConditionFunc(func(c *BuildContext) string {
		calls++
		return c.Quote("id") + "=" + c.Value(Param(0))
	}), Eq("data", fixed)).SetDialect(dialect.Postgres)
	q := mustCompileTemplate(t, b)
	wantSQL, args, err := q.Bind(1)
	if err != nil || calls != 1 {
		t.Fatal(err, calls)
	}

	args[0] = "overwritten"
	b.Reset().Select("other").From("different")
	for i := range 3 {
		s, a, err := q.Bind(i)
		if err != nil || s != wantSQL || a[0] != i || string(a[1].([]byte)) != "abc" || calls != 1 {
			t.Fatal(s, a, err, calls)
		}
	}

	// Constants retain the documented shallow ownership boundary.
	fixed[0] = 'z'
	_, a, _ := q.Bind(9)
	if string(a[1].([]byte)) != "zbc" {
		t.Fatal("unexpected deep copy")
	}

	static := mustCompileTemplate(t, Select("id").SetDialect(dialect.SQLite))
	if _, args, err := static.Bind(); err != nil || args != nil {
		t.Fatal(args, err)
	}
}

type templateTestExecutor struct {
	Executor
	query func(context.Context, string, ...any) (*sql.Rows, error)
	exec  func(context.Context, string, ...any) (sql.Result, error)
}

func (e templateTestExecutor) QueryContext(c context.Context, s string, a ...any) (*sql.Rows, error) {
	return e.query(c, s, a...)
}

func (e templateTestExecutor) ExecContext(c context.Context, s string, a ...any) (sql.Result, error) {
	return e.exec(c, s, a...)
}

func TestTemplateExecutionAndConfiguration(t *testing.T) {
	ctx := context.Background()
	fixture := &bindFixture{columns: []string{"id"}, values: [][]driver.Value{{int64(7)}}}
	db := bindTestDB(t, fixture).WithBindConfig(BindConfig{Capacity: 250})
	for _, tc := range []struct {
		name     string
		config   *BindConfig
		capacity int
	}{
		{"runtime_db", nil, 250},
		{"explicit_builder", &BindConfig{Capacity: 150}, 150},
		{"zero_builder_restores_hint", &BindConfig{}, 50},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Neither the original DB nor its executor is captured.
			b := (&DB{Dialect: dialect.SQLite}).Select("id").From("t").
				Where(Eq("id", Param(0))).Limit(50).SetExecutor(templateTestExecutor{})
			if tc.config != nil {
				b.SetBindConfig(*tc.config)
			}
			q := mustCompileTemplate(t, b)
			b.Limit(1).SetBindConfig(BindConfig{Capacity: 1})

			var borrowed []any
			exec := templateTestExecutor{query: func(c context.Context, s string, a ...any) (*sql.Rows, error) {
				if !strings.HasSuffix(s, "LIMIT 50") || !reflect.DeepEqual(a, []any{int64(42)}) {
					t.Fatal(s, a)
				}
				borrowed = a
				return db.QueryContext(c, s, a...)
			}}

			r := q.QueryRowsContext(ctx, db.WithExecutor(exec), int64(42))
			if len(borrowed) != 1 || borrowed[0] != nil {
				t.Fatal("query retained argument references", borrowed)
			}

			var got []int64
			if err := r.Bind(&got); err != nil || !slices.Equal(got, []int64{7}) || cap(got) != tc.capacity {
				t.Fatal(got, cap(got), err)
			}

			r = q.QueryRowsContext(ctx, db, int64(42)).SetBindConfig(BindConfig{Capacity: 2})
			if err := r.Bind(&got); err != nil || cap(got) != 2 {
				t.Fatal(got, cap(got), err)
			}
		})
	}

	// Frozen explicit policies own their mutable layouts, even after builder reuse.
	options := BindConfig{Scan: ScanOptions{TimeLayouts: []string{"2006-01-02"}}}
	b := db.Select("id").SetBindConfig(options)
	q := mustCompileTemplate(t, b)
	b.bconfig.Scan.TimeLayouts[0] = "changed"
	if q.config.Scan.TimeLayouts[0] != "2006-01-02" {
		t.Fatal("configuration snapshot aliases builder")
	}

	var got []int64
	q = mustCompileTemplate(t, db.Select("id"))
	err := q.QueryRowsContext(ctx, db.WithBindConfig(BindConfig{Capacity: 3})).Bind(&got)
	if err != nil || cap(got) != 3 {
		t.Fatal(got, err)
	}
}

func TestTemplateCapacityAndResultSets(t *testing.T) {
	ctx := context.Background()
	for _, n := range []int{0, 1, 101} {
		f := &bindFixture{columns: []string{"id"}}
		for i := range n {
			f.values = append(f.values, []driver.Value{int64(i)})
		}
		db := bindTestDB(t, f)
		q := mustCompileTemplate(t, db.Select("id").Limit(1000))

		var got []int64
		if err := q.QueryRowsContext(ctx, db).Bind(&got); err != nil || len(got) != n {
			t.Fatal(len(got), err)
		}
		if n == 0 && (got == nil || cap(got) != 0) || n == 1 && cap(got) != 100 {
			t.Fatal(got, cap(got))
		}
	}

	for _, explicit := range []int{0, 150} {
		std := sql.OpenDB(resultSetsConnector{&resultSetsRows{sets: []resultSetFixture{
			{columns: []string{"id"}},
			{columns: []string{"next"}, values: [][]driver.Value{{int64(7)}}},
		}}})
		t.Cleanup(func() { _ = std.Close() })
		db := (&DB{Dialect: dialect.SQLite, Executor: std}).WithBindConfig(BindConfig{Capacity: explicit})
		q := mustCompileTemplate(t, db.Select("id").Limit(3))
		r := q.QueryRowsContext(ctx, db)
		if !r.NextResultSet() {
			t.Fatal("missing result set", r.Err())
		}

		var got []int64
		if err := r.Bind(&got); err != nil {
			t.Fatal(err)
		}

		want := explicit
		if want == 0 {
			want = DefaultRowsCapacity
		}
		if len(got) != 1 || got[0] != 7 || cap(got) != want {
			t.Fatal(got, cap(got), want)
		}
	}
}

func TestTemplateExecutionModesAndCleanup(t *testing.T) {
	ctx := context.Background()
	f := &bindFixture{columns: []string{"id"}, values: [][]driver.Value{{int64(8)}}}
	db := bindTestDB(t, f)
	for _, b := range []templateBuilder{
		db.Select("id").Where(Eq("id", Param(0))),
		db.Insert().Into("t").Values(Param(0)).Returning("id"),
		db.Update().Table("t").Set(Set("id", Param(0))).Returning("id"),
		db.Delete().From("t").Where(Eq("id", Param(0))).Returning("id"),
	} {
		q := mustCompileTemplate(t, b)
		if _, err := q.ExecContext(ctx, db, 3); err == nil {
			t.Fatal("row-returning template accepted Exec")
		}

		var got []int64
		if err := q.QueryRowsContext(ctx, db, 3).Bind(&got); err != nil || !slices.Equal(got, []int64{8}) {
			t.Fatal(got, err)
		}
	}

	for _, b := range []templateBuilder{
		db.Insert().Into("t").Values(Param(0)),
		db.Update().Table("t").Set(Set("id", Param(0))),
		db.Delete().From("t").Where(Eq("id", Param(0))),
	} {
		q := mustCompileTemplate(t, b)
		if err := q.QueryRowsContext(ctx, db, 3).Err(); err == nil {
			t.Fatal("non-returning DML accepted Query")
		}

		for _, mode := range []string{"success", "error", "panic"} {
			var borrowed []any
			errExec := errors.New("exec failed")
			e := templateTestExecutor{exec: func(_ context.Context, _ string, a ...any) (sql.Result, error) {
				borrowed = a
				if !reflect.DeepEqual(a, []any{3}) {
					t.Fatal(a)
				}

				switch mode {
				case "error":
					return nil, errExec
				case "panic":
					panic(errExec)
				}

				return driver.RowsAffected(1), nil
			}}

			func() {
				defer func() {
					if r := recover(); r != nil {
						if mode != "panic" || r != errExec {
							t.Fatal(r)
						}
					} else if mode == "panic" {
						t.Error("missing executor panic")
					}
				}()

				_, err := q.ExecContext(ctx, db.WithExecutor(e), 3)
				if mode == "error" && !errors.Is(err, errExec) || mode == "success" && err != nil {
					t.Fatal(err)
				}
			}()

			if len(borrowed) != 1 || borrowed[0] != nil {
				t.Fatal("executor references retained", borrowed)
			}
		}
	}
}

// A comparable outer type can still contain an uncomparable dynamic value.
type templateValueDialect struct {
	Dialect
	options any
}

func TestTemplateDialectCompatibility(t *testing.T) {
	base := dialect.SQLite
	rules := base.LexicalRules()
	rules.BackslashStrings = !rules.BackslashStrings
	for _, tc := range []struct {
		name              string
		compiled, execute Dialect
		compatible        bool
	}{
		{"builtin", base, base, true},
		{"other_builtin", base, dialect.MySQL, false},
		{"same_name_different_rules", base, dialect.WithLexicalRules(base, rules), false},
		{"equal_configurations", dialect.WithLexicalRules(base, rules), dialect.WithLexicalRules(base, rules), true},
		{"uncomparable_equal", templateValueDialect{base, []int{1}}, templateValueDialect{base, []int{1}}, true},
		{"uncomparable_different", templateValueDialect{base, []int{1}}, templateValueDialect{base, []int{2}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			q := mustCompileTemplate(t, Insert().Into("t").Values(Param(0)).SetDialect(tc.compiled))
			called := false
			db := &DB{
				Dialect: tc.execute,
				Executor: recordingExecutor{
					call: func(string, []any) {
						called = true
					},
				},
			}

			_, err := q.ExecContext(context.Background(), db, 1)
			if (err == nil) != tc.compatible || called != tc.compatible {
				t.Fatal(err, called)
			}
		})
	}
}

type templateValuer struct{ calls *int }

func (v templateValuer) Value() (driver.Value, error) {
	*v.calls += 1
	return int64(4), nil
}

func TestTemplateValuerAndQueryErrors(t *testing.T) {
	ctx := context.Background()
	db := bindTestDB(t, &bindFixture{columns: []string{"id"}})
	calls := 0
	v := templateValuer{&calls}
	q := mustCompileTemplate(t, db.Select("id").Where(Eq("a", v), Eq("b", Param(0))))
	if _, _, err := q.Bind(v); err != nil || calls != 0 {
		t.Fatal(calls, err)
	}

	r := q.QueryRowsContext(ctx, db, v)
	if err := r.Err(); err != nil || calls != 2 {
		t.Fatal(calls, err)
	}
	_ = r.Close()

	if err := q.QueryRowsContext(ctx, nil, v).Err(); err == nil {
		t.Fatal("nil DB accepted")
	}

	for _, failure := range []error{nil, context.Canceled, errors.New("query failure")} {
		var borrowed []any
		e := templateTestExecutor{query: func(_ context.Context, _ string, a ...any) (*sql.Rows, error) {
			borrowed = a
			return nil, failure
		}}

		err := q.QueryRowsContext(ctx, db.WithExecutor(e), v).Err()
		if err == nil || failure != nil && !errors.Is(err, failure) {
			t.Fatal(err)
		}

		for _, a := range borrowed {
			if a != nil {
				t.Fatal("query error retained argument", a)
			}
		}
	}
}

func TestTemplateConcurrentExecution(t *testing.T) {
	f := &bindFixture{columns: []string{"id"}, values: [][]driver.Value{{int64(1)}}}
	db := bindTestDB(t, f)
	q := mustCompileTemplate(t, db.Select("id").Where(In("id", Param(0), Param(0))).Limit(1))

	type requestKey struct{}
	runner := db.WithExecutor(templateTestExecutor{
		query: func(ctx context.Context, sql string, args ...any) (*sql.Rows, error) {
			want := ctx.Value(requestKey{})
			if len(args) != 2 || args[0] != want || args[1] != want {
				return nil, fmt.Errorf("request %v received arguments %v", want, args)
			}
			return db.QueryContext(ctx, sql, args...)
		},
	})

	var wg sync.WaitGroup
	for worker := range 16 {
		wg.Go(func() {
			for i := range 20 {
				v := worker*20 + i
				s, a, err := q.Bind(v)
				if err != nil || s == "" || !reflect.DeepEqual(a, []any{v, v}) {
					t.Error(s, a, err)
					return
				}

				a[0] = "changed"
				var got []int64
				ctx := context.WithValue(context.Background(), requestKey{}, v)
				if err := q.QueryRowsContext(ctx, runner, v).Bind(&got); err != nil || len(got) != 1 {
					t.Error(got, err)
					return
				}
			}
		})
	}
	wg.Wait()
}

func ExampleStatementTemplate() {
	query, err := Select("id", "name").From("users").
		Where(Eq("tenant_id", Param(0))).Limit(100).
		SetDialect(dialect.Postgres).Compile()
	if err != nil {
		panic(err)
	}

	sql, args, err := query.Bind(int64(42))
	if err != nil {
		panic(err)
	}

	fmt.Println(sql)
	fmt.Println(args)
	// Output:
	// SELECT "id", "name" FROM "users" WHERE ("tenant_id" = $1) LIMIT 100
	// [42]
}
