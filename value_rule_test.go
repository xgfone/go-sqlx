// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"reflect"
	"sync"
	"testing"

	"github.com/xgfone/go-sqlx/dialect"
	"github.com/xgfone/go-sqlx/sqltype"
)

func TestRuleColumnExecution(t *testing.T) {
	ctx := context.Background()
	name := Column("name").WithValueRule(sqltype.StringLimit{
		Max:      2,
		Overflow: sqltype.Truncate,
	})

	var got []any
	calls := 0
	db := &DB{Dialect: dialect.Postgres, Executor: templateTestExecutor{
		exec: func(_ context.Context, _ string, args ...any) (sql.Result, error) {
			calls++
			got = append([]any(nil), args...)
			return driver.RowsAffected(1), nil
		},
	}}

	for _, setter := range []Updater{name.Set("你好吗"), Set(name, "你好吗")} {
		q := db.Update().Table("users").Set(setter).Where(name.Eq("unchanged"))
		_, args, err := q.Build()
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := args[0].(driver.Valuer); !ok {
			t.Fatalf("Build resolved value: %#v", args)
		}

		for range 2 {
			if _, err = q.ExecContext(ctx); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, []any{"你好", "unchanged"}) {
				t.Fatal(got)
			}
		}
	}

	_, err := db.Insert().Into("users").Row(ColValue(name, "你好呀")).ExecContext(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, []any{"你好"}) {
		t.Fatal(got)
	}

	before := calls
	strict := name.WithValueRule(sqltype.StringLimit{Max: 1})
	_, err = db.Update().Table("users").Set(strict.Set("你好")).ExecContext(ctx)
	if _, ok := errors.AsType[*sqltype.StringLengthError](err); !ok || calls != before {
		t.Fatalf("error=%v, calls=%d", err, calls)
	}

	// Assignment expressions bypass the client rule; explicit values do not.
	_, err = db.Update().Table("users").Set(
		strict.Set(name.Ref()),
		name.Set(Value("你好呀")),
		name.Set(nil),
	).ExecContext(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, []any{"你好", nil}) {
		t.Fatal(got)
	}
}

func TestRuleColumnComparisons(t *testing.T) {
	name := Column("name").WithValueRule(sqltype.StringLimit{
		Max:      2,
		Overflow: sqltype.Truncate,
	}).WithComparisons(true)
	u := name.Scope("u")

	q := SelectColumns(Column("id")).SelectColumns(u).Where(
		u.Eq("abcd"), Eq(u, "efgh"), u.Between("ijkl", "mnop"),
		In(u, "qrst", "uvwx", nil), u.Like("long%pattern"),
		u.Eq(nil), u.On(Column("name").Scope("v")),
	).SetDialect(dialect.Postgres)

	_, args, err := q.Build()
	if err != nil {
		t.Fatal(err)
	}
	if err = resolveRuleArgs(args); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(args, []any{"ab", "ef", "ij", "mn", "qr", "uv", nil, "long%pattern"}) {
		t.Fatal(args)
	}

	checkSQL(t,
		SelectColumns(u).Where(u.Eq(nil), u.IsNotNull()).SetDialect(dialect.Postgres),
		`SELECT "u"."name" WHERE (("u"."name" IS NULL) AND ("u"."name" IS NOT NULL))`)
	if name.Name() != "name" || u.Name() != "u.name" {
		t.Fatal(name, u)
	}

	_, args, err = SelectColumns(name).Where(name.WithComparisons(false).Eq("abcd")).Build()
	if err != nil || !reflect.DeepEqual(args, []any{"abcd"}) {
		t.Fatal(args, err)
	}
}

func TestRuleColumnPromotedOperations(t *testing.T) {
	rule := ValueRuleFunc(func(any) (any, error) {
		t.Fatal("a promoted operation must not apply the rule")
		return nil, nil
	})

	c := Column("name").WithValueRule(rule).WithComparisons(true).Scope("u")
	checkSQL(t,
		SelectColumns(c).Where(c.Like("long%pattern"), c.IsNotNull(), c.On(Column("name").Scope("v"))).
			Sort(c.Asc()).SetDialect(dialect.Postgres),
		`SELECT "u"."name" WHERE (("u"."name" LIKE $1) AND ("u"."name" IS NOT NULL) AND "u"."name"="v"."name") ORDER BY "u"."name" ASC`, "long%pattern")
	checkSQL(t,
		Select().SelectExpr(c.Ref(), c.Count(), c.Sum(), c.Cast("TEXT"), c.NullIf("value")).SetDialect(dialect.Postgres),
		`SELECT "u"."name", COUNT("u"."name"), SUM("u"."name"), CAST("u"."name" AS TEXT), NULLIF("u"."name", $1)`, "value")
	checkSQL(t,
		Select().SelectNamers(c.As("display_name")).SetDialect(dialect.Postgres),
		`SELECT "u"."name" AS "display_name"`)
}

func TestRuleTemplateOccurrenceAndConcurrency(t *testing.T) {
	a := Column("a").WithValueRule(sqltype.StringLimit{Max: 1, Overflow: sqltype.Truncate})
	b := Column("b").WithValueRule(sqltype.StringLimit{Max: 2, Overflow: sqltype.Truncate})
	q := mustCompileTemplate(t, Update().Table("t").Set(a.Set(Param(0)), b.Set(Param(0))).SetDialect(dialect.Postgres))
	db := &DB{Dialect: dialect.Postgres, Executor: templateTestExecutor{
		exec: func(_ context.Context, _ string, args ...any) (sql.Result, error) {
			if !reflect.DeepEqual(args, []any{"你", "你好"}) {
				t.Errorf("args: %#v", args)
			}
			return driver.RowsAffected(1), nil
		},
	}}

	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 5 {
				if _, err := q.ExecContext(context.Background(), db, "你好呀"); err != nil {
					t.Error(err)
				}
			}
		})
	}
	wg.Wait()

	_, args, err := q.Bind("你好呀")
	if err != nil {
		t.Fatal(err)
	}

	for i, want := range []string{"你", "你好"} {
		v, err := args[i].(driver.Valuer).Value()
		if err != nil || v != want {
			t.Fatal(v, err)
		}
	}

	// Unbound and invalid wrapped slots obey the ordinary Param validation.
	checkBuildError(t, Update().Table("t").Set(a.Set(Param(0))))
	for _, v := range []any{Param(-1), Param(2), sql.Named("bad", Param(0))} {
		if _, err := Update().Table("t").Set(a.Set(v)).Compile(); err == nil {
			t.Fatalf("accepted %#v", v)
		}
	}
}

func TestRuleQueryFailureBeforeExecutor(t *testing.T) {
	sentinel := errors.New("rule failed")
	applyCalls, queryCalls := 0, 0
	name := Column("name").WithValueRule(ValueRuleFunc(func(any) (any, error) {
		applyCalls++
		return nil, sentinel
	})).WithComparisons(true)
	db := &DB{Dialect: dialect.Postgres, Executor: templateTestExecutor{
		query: func(context.Context, string, ...any) (*sql.Rows, error) {
			queryCalls++
			return nil, nil
		},
	}}

	q := db.SelectColumns(name).Where(name.Eq(Param(0)))
	tmpl := mustCompileTemplate(t, q)
	if _, _, err := tmpl.Bind("value"); err != nil {
		t.Fatal(err)
	}
	if applyCalls != 0 {
		t.Fatal("rule ran during Compile or Bind")
	}

	for _, err := range []error{
		tmpl.QueryRowsContext(context.Background(), db, "value").Err(),
		db.SelectColumns(name).Where(name.Eq("value")).
			QueryRowContext(context.Background()).Err(),
		db.Update().Table("t").Set(name.Set("value")).Returning("name").
			QueryRowsContext(context.Background()).Err(),
	} {
		if !errors.Is(err, sentinel) {
			t.Fatal(err)
		}
	}
	if queryCalls != 0 || applyCalls != 3 {
		t.Fatal(queryCalls, applyCalls)
	}
}

func TestRuleNamedValuesAndFallback(t *testing.T) {
	rule := sqltype.StringLimit{Max: 2, Overflow: sqltype.Truncate}
	c := Column("name").WithValueRule(rule)
	_, args, err := Update().Table("t").Set(c.Set(sql.Named("n", "abcd"))).
		SetDialect(dialect.SQLite).Build()
	if err != nil {
		t.Fatal(err)
	}

	if err = resolveRuleArgs(args); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(args, []any{sql.Named("n", "ab")}) {
		t.Fatal(args)
	}

	v := WithValueRule(sql.NullString{String: "abcd", Valid: true}, rule).(driver.Valuer)
	if got, err := v.Value(); err != nil || got != "ab" {
		t.Fatal(got, err)
	}

	v = WithValueRule(sql.NullString{}, rule).(driver.Valuer)
	if got, err := v.Value(); err != nil || got != nil {
		t.Fatal(got, err)
	}

	v = WithValueRule("abcd", ValueRuleFunc(nil)).(driver.Valuer)
	if _, err := v.Value(); err == nil {
		t.Fatal("nil rule accepted")
	}

	// Explicit wrappers compose from the inside out.
	v = WithValueRule(WithValueRule("abcd", rule), sqltype.StringLimit{
		Max:      1,
		Overflow: sqltype.Truncate,
	}).(driver.Valuer)
	if got, err := v.Value(); err != nil || got != "a" {
		t.Fatal(got, err)
	}
}

func TestRuleBuiltArgsDatabaseSQL(t *testing.T) {
	// Build's deferred args must work through the standard Valuer path as well
	// as through sqlx's executor normalization.
	f := &txFixture{}
	std := sql.OpenDB(txConnector{f})
	defer std.Close() //nolint:errcheck

	name := Column("name").WithValueRule(sqltype.StringLimit{Max: 1})
	query, args, err := Update().Table("t").Set(name.Set("too long")).Build()
	if err != nil {
		t.Fatal(err)
	}

	_, err = std.ExecContext(context.Background(), query, args...)
	var length *sqltype.StringLengthError
	if !errors.As(err, &length) || f.execs != 0 {
		t.Fatal(err, f.execs)
	}

	query, args, err = Update().Table("t").Set(name.Set("a")).Build()
	if err != nil {
		t.Fatal(err)
	}

	_, err = std.ExecContext(context.Background(), query, args...)
	if err != nil || f.execs != 1 {
		t.Fatal(err, f.execs)
	}
}

func TestRuleDoesNotConvertUnrelatedValuers(t *testing.T) {
	calls := 0
	v := templateValuer{calls: &calls}
	name := Column("name").WithValueRule(sqltype.StringLimit{Max: 1})
	args := []any{v, sql.Named("v", v), withValueRule("a", name.rule, name.Name())}
	if err := resolveRuleArgs(args); err != nil {
		t.Fatal(err)
	}
	if calls != 0 || !reflect.DeepEqual(args, []any{v, sql.Named("v", v), "a"}) {
		t.Fatal(args, calls)
	}
}
