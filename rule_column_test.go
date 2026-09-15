// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/xgfone/go-sqlx/dialect"
	"github.com/xgfone/go-sqlx/sqltype"
)

func TestRuleColumnEagerAssignments(t *testing.T) {
	calls := 0
	limit := sqltype.StringLimit{Max: 2, Overflow: sqltype.Truncate}
	name := Column("name").WithValueRule(ValueRuleFunc(func(v any) (any, error) {
		calls++
		return limit.Apply(v)
	}))

	setter, row := name.Set("你好吗"), name.ColValue(Value("你好吗"))
	if calls != 2 || row.Value != "你好" {
		t.Fatal("rules must run at assignment", calls, row)
	}

	db := &DB{Dialect: dialect.Postgres, Executor: templateTestExecutor{
		exec: func(_ context.Context, _ string, args ...any) (sql.Result, error) {
			if !reflect.DeepEqual(args, []any{"你好"}) {
				t.Errorf("executor received unresolved args: %#v", args)
			}
			return driver.RowsAffected(1), nil
		},
	}}

	update := db.Update().Table("users").Set(setter)
	insert := db.Insert().Into("users").Row(row)
	for _, b := range []interface {
		SQLBuilder
		Compile() (*StatementTemplate, error)
		ExecContext(context.Context) (sql.Result, error)
	}{update, update.Clone(), insert, insert.Clone()} {
		for range 2 {
			_, args, err := b.Build()
			if err != nil || !reflect.DeepEqual(args, []any{"你好"}) {
				t.Fatal(args, err)
			}
			if _, err = b.ExecContext(context.Background()); err != nil {
				t.Fatal(err)
			}
		}

		tmpl, err := b.Compile()
		if err != nil {
			t.Fatal(err)
		}

		_, args, err := tmpl.Bind()
		if err != nil || !reflect.DeepEqual(args, []any{"你好"}) {
			t.Fatal(args, err)
		}

		if _, err = tmpl.ExecContext(context.Background(), db); err != nil {
			t.Fatal(err)
		}
	}
	if calls != 2 {
		t.Fatal("rule ran after assignment", calls)
	}
}

func TestRuleColumnErrors(t *testing.T) {
	sentinel := &sqltype.StringLengthError{Max: 1, Actual: 2}
	calls := 0
	name := Column("name").WithValueRule(ValueRuleFunc(func(any) (any, error) {
		calls++
		return nil, sentinel
	}))

	setter, row := name.Set("ab"), name.ColValue("ab")
	if calls != 2 {
		t.Fatal("failed rules must also run immediately", calls)
	}

	checkError := func(err error) {
		t.Helper()
		var length *sqltype.StringLengthError
		if !errors.Is(err, sentinel) || !errors.As(err, &length) || !strings.Contains(err.Error(), `column "name"`) {
			t.Fatalf("lost rule error or column context: %v", err)
		}
	}

	db := &DB{Dialect: dialect.Postgres, Executor: templateTestExecutor{
		exec: func(context.Context, string, ...any) (sql.Result, error) {
			t.Error("invalid assignment reached executor")
			return nil, nil
		},
		query: func(context.Context, string, ...any) (*sql.Rows, error) {
			t.Error("invalid assignment reached query executor")
			return nil, nil
		},
	}}

	update := db.Update().Table("users").Set(setter)
	insert := db.Insert().Into("users").Row(ColValue("id", 1), row)
	if len(insert.columns) != 0 || insert.values.rows != 0 {
		t.Fatal("failed Row changed columns or appended data")
	}

	for _, b := range []interface {
		SQLBuilder
		Compile() (*StatementTemplate, error)
		ExecContext(context.Context) (sql.Result, error)
	}{
		update, update.Clone(), insert, insert.Clone(),
		db.Update().Table("users").Set(Batch(Set("id", 1), setter)),
		db.Insert().Into("users").Row(ColValue("id", 1)).OnConflict(ConflictColumns("id").DoUpdate(setter)),
		db.Insert().Into("users").Row(ColValue("id", 1)).OnDuplicateKeyUpdate(setter).SetDialect(dialect.MySQL),
	} {
		query, args, err := b.Build()
		checkError(err)
		if query != "" || len(args) != 0 {
			t.Fatal("failed Build returned partial SQL or arguments", query, args)
		}

		_, err = b.Compile()
		checkError(err)

		_, err = b.ExecContext(context.Background())
		checkError(err)
	}
	checkError(update.Returning("name").QueryRowsContext(context.Background()).Err())
	checkError(insert.Returning("name").QueryRowsContext(context.Background()).Err())
	if calls != 2 {
		t.Fatal("failed rules were retried", calls)
	}

	// Failure belongs to the assignment/row, never to the reusable writer.
	strict := Column("name").WithValueRule(sqltype.StringLimit{Max: 1})
	if _, err := strict.Apply("ab"); err == nil {
		t.Fatal("Apply must return its error directly")
	}

	checkSQL(t,
		Update().Table("users").Set(strict.Set("a")).SetDialect(dialect.Postgres),
		`UPDATE "users" SET "name"=$1`, "a")
}

func TestRuleColumnInputValues(t *testing.T) {
	type text string
	var nilString *string
	name := Column("name").WithValueRule(sqltype.StringLimit{Max: 2, Overflow: sqltype.Truncate})
	for _, tc := range []struct {
		input any
		want  any
	}{
		{"abcd", "ab"}, {text("abcd"), "ab"}, {Value("abcd"), "ab"},
		{nil, nil}, {nilString, nil}, {Value(nil), nil},
		{sql.Named("n", "abcd"), sql.Named("n", "ab")},
		{sql.Named("n", Value("abcd")), sql.Named("n", "ab")},
		{Value(sql.Named("n", "abcd")), sql.Named("n", "ab")},
		{sql.Named("n", nil), sql.Named("n", nil)},
	} {
		got, err := name.Apply(tc.input)
		if err != nil || !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("Apply(%#v) = %#v, %v; want %#v", tc.input, got, err, tc.want)
		}

		row := name.ColValue(tc.input)
		checkSQL(t, Insert().Into("users").Row(row).SetDialect(dialect.SQLite),
			insertRuleSQL(tc.want), tc.want)
	}
}

func insertRuleSQL(value any) string {
	if v, ok := value.(sql.NamedArg); ok && v.Name != "" {
		return `INSERT INTO "users" ("name") VALUES (@` + v.Name + `)`
	}
	return `INSERT INTO "users" ("name") VALUES (?)`
}

func TestRuleColumnRejectsSQLInputs(t *testing.T) {
	name := Column("name").WithValueRule(ValueRuleFunc(func(any) (any, error) {
		t.Fatal("SQL input reached rule")
		return nil, nil
	}))
	for _, value := range []any{
		Param(0), Param(-1), Value(Param(0)), sql.Named("n", Param(0)),
		Default(), name.Column().Ref(), Expr("UPPER(?)", "abcd"),
		Subquery(Select("name").From("users")), Select("name"), templateParam(0),
	} {
		if _, err := name.Apply(value); err == nil {
			t.Fatalf("Apply accepted SQL input %T", value)
		}

		for _, b := range []interface {
			SQLBuilder
			Compile() (*StatementTemplate, error)
		}{
			Update().Table("users").Set(name.Set(value)),
			Insert().Into("users").Row(name.ColValue(value)),
		} {
			checkBuildError(t, b)
			if _, err := b.Compile(); err == nil {
				t.Fatalf("Compile accepted SQL input %T", value)
			}
		}
	}

	// Expression assignments remain available explicitly through the base column.
	checkSQL(t,
		Update().Table("users").Set(name.Column().Set(Expr("UPPER(?)", "abcd"))).SetDialect(dialect.Postgres),
		`UPDATE "users" SET "name"=UPPER($1)`, "abcd")
}

func TestRuleColumnRuleContract(t *testing.T) {
	for _, rule := range []ValueRule{nil, ValueRuleFunc(nil), (*sqltype.StringLimit)(nil)} {
		name := Column("name").WithValueRule(rule)
		if _, err := name.Apply("a"); err == nil {
			t.Fatal("nil rule accepted non-nil data")
		}
		if v, err := name.Apply(nil); v != nil || err != nil {
			t.Fatal("nil must bypass rules", v, err)
		}
	}

	for _, result := range []any{
		Expr("1"), Value("a"), Param(0), Select("id"),
		sql.Named("n", "a"), templateParam(0),
	} {
		name := Column("name").WithValueRule(ValueRuleFunc(func(any) (any, error) {
			return result, nil
		}))
		if _, err := name.Apply("a"); err == nil {
			t.Fatalf("rule returned non-data value %T", result)
		}
	}

	name := Column("name").WithValueRule(sqltype.StringLimit{Max: 2})
	if _, err := name.Apply(sql.Named("a", sql.Named("b", "x"))); err == nil {
		t.Fatal("nested named arguments accepted")
	}
}

func TestRuleColumnNamedReuse(t *testing.T) {
	name := Column("name").WithValueRule(ValueRuleFunc(func(v any) (any, error) {
		return strings.ToUpper(v.(string)), nil
	}))

	setter := name.Set(sql.Named("n", "alice"))
	checkSQL(t,
		Update().Table("users").Set(setter).Where(name.Column().Eq(sql.Named("n", "ALICE"))).SetDialect(dialect.SQLite),
		`UPDATE "users" SET "name"=@n WHERE ("name" = @n)`, sql.Named("n", "ALICE"))

	// The same rule may also be used for two different columns sharing a name.
	other := Column("other").WithValueRule(name.rule)
	checkSQL(t,
		Update().Table("users").
			Set(setter, other.Set(sql.Named("n", "alice"))).
			SetDialect(dialect.SQLite),
		`UPDATE "users" SET "name"=@n, "other"=@n`, sql.Named("n", "ALICE"))
	checkBuildError(t,
		Update().Table("users").Set(setter).
			Where(name.Column().Eq(sql.Named("n", "different"))).
			SetDialect(dialect.SQLite))
}

func TestRuleColumnDoesNotConvertValuers(t *testing.T) {
	calls := 0
	valuer := templateValuer{calls: &calls}
	name := Column("name").WithValueRule(ValueRuleFunc(func(value any) (any, error) {
		if value != valuer {
			t.Fatalf("rule did not receive original Valuer: %#v", value)
		}
		return value, nil
	}))

	q := Update().Table("users").Set(name.Set(valuer), Set("other", valuer))
	_, args, err := q.Build()
	if err != nil || calls != 0 || !reflect.DeepEqual(args, []any{valuer, valuer}) {
		t.Fatal(args, err, calls)
	}

	strict := name.Column().WithValueRule(sqltype.StringLimit{Max: 2})
	if _, err := strict.Apply(valuer); err == nil || calls != 0 {
		t.Fatal("StringLimit must reject Valuers without converting them", err, calls)
	}
}

func TestRuleColumnTemplateInputs(t *testing.T) {
	name := Column("name").WithValueRule(sqltype.StringLimit{Max: 2, Overflow: sqltype.Truncate})
	strict := name.Column().WithValueRule(sqltype.StringLimit{Max: 2})
	if _, err := strict.Apply("你好呀"); err == nil {
		t.Fatal("scenario-specific writer lost its policy")
	}

	tmpl := mustCompileTemplate(t, Update().Table("users").Set(name.Column().Set(Param(0))).
		SetDialect(dialect.Postgres))
	db := &DB{Dialect: dialect.Postgres, Executor: templateTestExecutor{
		exec: func(_ context.Context, _ string, args ...any) (sql.Result, error) {
			if !reflect.DeepEqual(args, []any{"你好"}) {
				t.Errorf("template args: %#v", args)
			}
			return driver.RowsAffected(1), nil
		},
	}}

	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			value, err := name.Apply("你好呀")
			if err != nil {
				t.Error(err)
				return
			}

			_, args, err := tmpl.Bind(value)
			if err != nil || !reflect.DeepEqual(args, []any{"你好"}) {
				t.Error(args, err)
			}

			if _, err = tmpl.ExecContext(context.Background(), db, value); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
}
