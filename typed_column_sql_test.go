// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"database/sql"
	"reflect"
	"testing"

	"github.com/xgfone/go-sqlx/dialect"
)

func TestTypedColumnSelectionJoinOrdering(t *testing.T) {
	type userID int64
	id := NewTypedColumn[userID]("id").Scope("u")
	owner := NewTypedColumn[userID]("user_id").Scope("o")
	name := NewTypedColumn[string]("name").Scope("u")
	if id.Asc() != id.Column().Asc() || owner.Desc() != owner.Column().Desc() ||
		name.As("user.name") != name.Column().As("user.name") {
		t.Fatal("typed selection and ordering must preserve the underlying column terms")
	}
	for _, tc := range []struct {
		d    Dialect
		want string
	}{
		{dialect.MySQL, "SELECT `u`.`name` AS `user.name` FROM `users` AS `u` INNER JOIN `orders` AS `o` ON `u`.`id`=`o`.`user_id` WHERE (`u`.`id` = ?) ORDER BY `u`.`id` ASC, `o`.`user_id` DESC, `u`.`name` ASC"},
		{dialect.Postgres, `SELECT "u"."name" AS "user.name" FROM "users" AS "u" INNER JOIN "orders" AS "o" ON "u"."id"="o"."user_id" WHERE ("u"."id" = $1) ORDER BY "u"."id" ASC, "o"."user_id" DESC, "u"."name" ASC`},
		{dialect.SQLite, `SELECT "u"."name" AS "user.name" FROM "users" AS "u" INNER JOIN "orders" AS "o" ON "u"."id"="o"."user_id" WHERE ("u"."id" = ?) ORDER BY "u"."id" ASC, "o"."user_id" DESC, "u"."name" ASC`},
	} {
		t.Run(tc.d.Name(), func(t *testing.T) {
			q := Select().SelectNamers(name.As("user.name")).FromAlias("users", "u").
				Join("orders", "o", id.On(owner)).Where(id.Eq(7)).
				Sort(id.Asc(), owner.Desc(), name.Asc()).SetDialect(tc.d)
			checkSQL(t, q, tc.want, userID(7))
		})
	}

	quoted := NewTypedColumn[*string](`n"ame`).Scope(`u"x`)
	other := NewTypedColumn[*string]("backup").Scope("o")
	checkSQL(t, Select().SelectNamers(quoted.As(`result."name`)).
		FromAlias("users", `u"x`).Join("other", "o", quoted.On(other)).Sort(quoted.Desc()).
		SetDialect(dialect.Postgres),
		`SELECT "u""x"."n""ame" AS "result.""name" FROM "users" AS "u""x" INNER JOIN "other" AS "o" ON "u""x"."n""ame"="o"."backup" ORDER BY "u""x"."n""ame" DESC`)
	if name.Name() != "u.name" || name.Column().Typed[string]() != name {
		t.Fatal("As must not replace the original path or its type")
	}
}

func TestTypedColumnOrderingEntrypoints(t *testing.T) {
	age := NumericColumn[int64]("age")
	name := NewTypedColumn[string]("name")
	checkSQL(t, Select().SelectExpr(RowNumber().Over(Window().Sort(age.Desc(), name.Asc()))).
		From("users").SetDialect(dialect.Postgres),
		`SELECT ROW_NUMBER() OVER (ORDER BY "age" DESC, "name" ASC) FROM "users"`)
	type record struct {
		Age  int64  `sql:"age"`
		Name string `sql:"name"`
	}
	o := NewOper[record]("users").WithDB(&DB{Dialect: dialect.Postgres}).
		WithStructSorter(SortColumns{age.Desc(), name.Asc()})
	checkSQL(t, o.SelectStruct(), `SELECT "age", "name" FROM "users" ORDER BY "age" DESC, "name" ASC`)
	checkSQL(t, Update().Table("users").Set(age.Set(18)).Sort(age.Desc()).Limit(1).SetDialect(dialect.MySQL),
		"UPDATE `users` SET `age`=? ORDER BY `age` DESC LIMIT 1", int64(18))
	checkSQL(t, Delete().From("users").Sort(age.Asc()).Limit(1).SetDialect(dialect.MySQL),
		"DELETE FROM `users` ORDER BY `age` ASC LIMIT 1")

	term := NewTypedColumn[[]byte]("data").Desc()
	term.Nulls = NullsLast
	for _, d := range []Dialect{dialect.Postgres, dialect.SQLite} {
		checkSQL(t, Select("data").From("users").Sort(term).SetDialect(d),
			`SELECT "data" FROM "users" ORDER BY "data" DESC NULLS LAST`)
	}
	q := Select("data").From("users").Sort(term).SetDialect(dialect.MySQL)
	checkBuildError(t, q)
	if _, err := q.Compile(); err == nil {
		t.Fatal("typed sort terms must retain dialect checks for NULL placement")
	}
}

func TestTypedColumnAggregateExpressions(t *testing.T) {
	amount := NewTypedColumn[float64]("amount").Scope("o")
	for _, tc := range []struct {
		d    Dialect
		want string
	}{
		{dialect.MySQL, "SELECT COUNT(`o`.`amount`), COUNT(DISTINCT `o`.`amount`), SUM(`o`.`amount`), MIN(`o`.`amount`), MAX(`o`.`amount`), AVG(`o`.`amount`), CAST(`o`.`amount` AS DECIMAL(12,2)) FROM `orders` AS `o` GROUP BY `o`.`amount`"},
		{dialect.Postgres, `SELECT COUNT("o"."amount"), COUNT(DISTINCT "o"."amount"), SUM("o"."amount"), MIN("o"."amount"), MAX("o"."amount"), AVG("o"."amount"), CAST("o"."amount" AS DECIMAL(12,2)) FROM "orders" AS "o" GROUP BY "o"."amount"`},
		{dialect.SQLite, `SELECT COUNT("o"."amount"), COUNT(DISTINCT "o"."amount"), SUM("o"."amount"), MIN("o"."amount"), MAX("o"."amount"), AVG("o"."amount"), CAST("o"."amount" AS DECIMAL(12,2)) FROM "orders" AS "o" GROUP BY "o"."amount"`},
	} {
		t.Run(tc.d.Name(), func(t *testing.T) {
			checkSQL(t, Select().SelectExpr(amount.Count(), amount.CountDistinct(), amount.Sum(),
				amount.Min(), amount.Max(), amount.Avg(), amount.Cast("DECIMAL(12,2)")).
				FromAlias("orders", "o").GroupBy("o.amount").SetDialect(tc.d), tc.want)
		})
	}
	name := NewTypedColumn[string]("name")
	checkSQL(t, Select().SelectExpr(name.Min(), name.Max(), name.CountDistinct()).From("users").SetDialect(dialect.Postgres),
		`SELECT MIN("name"), MAX("name"), COUNT(DISTINCT "name") FROM "users"`)
	checkSQL(t, Select().SelectExpr(amount.Sum().Filter(amount.Gt(0)).Over(Window().Sort(amount.Asc()))).
		FromAlias("orders", "o").SetDialect(dialect.Postgres),
		`SELECT SUM("o"."amount") FILTER (WHERE ("o"."amount" > $1)) OVER (ORDER BY "o"."amount" ASC) FROM "orders" AS "o"`, float64(0))
}

func TestTypedColumnCoalesceAndNullIf(t *testing.T) {
	type amount int64
	c := NewTypedColumn[amount]("amount").Scope("o")
	values := []amount{0, 1}
	exprs := []Expression{NewTypedColumn[amount]("backup").Ref(), Value(2)}
	coalesce, coalesceExpr := c.Coalesce(values...), c.CoalesceExpr(exprs...)
	clear(values)
	clear(exprs)
	for _, tc := range []struct {
		d    Dialect
		want string
	}{
		{dialect.MySQL, "SELECT COALESCE(`o`.`amount`, ?, ?), COALESCE(`o`.`amount`, `backup`, ?), NULLIF(`o`.`amount`, ?), NULLIF(`o`.`amount`, `backup`) FROM `orders` AS `o`"},
		{dialect.Postgres, `SELECT COALESCE("o"."amount", $1, $2), COALESCE("o"."amount", "backup", $3), NULLIF("o"."amount", $4), NULLIF("o"."amount", "backup") FROM "orders" AS "o"`},
		{dialect.SQLite, `SELECT COALESCE("o"."amount", ?, ?), COALESCE("o"."amount", "backup", ?), NULLIF("o"."amount", ?), NULLIF("o"."amount", "backup") FROM "orders" AS "o"`},
	} {
		t.Run(tc.d.Name(), func(t *testing.T) {
			q := Select().SelectExpr(coalesce, coalesceExpr, c.NullIf(0), c.NullIfExpr(Column("backup").Ref())).
				FromAlias("orders", "o").SetDialect(tc.d)
			want := []any{amount(0), amount(1), 2, amount(0)}
			checkSQL(t, q, tc.want, want...)
			_, args, err := mustCompileTemplate(t, q).Bind()
			if err != nil || !reflect.DeepEqual(args, want) {
				t.Fatal(args, err)
			}
		})
	}
}

func TestTypedColumnFunctionData(t *testing.T) {
	var nilValue *string
	fallback := "fallback"
	c := NewTypedColumn[*string]("v")
	e := c.Coalesce(nil, &fallback)
	fallback = "changed"
	checkSQL(t, Select().SelectExpr(e, c.NullIf(nil), c.CoalesceExpr(Value("unknown"))).SetDialect(dialect.Postgres),
		`SELECT COALESCE("v", $1, $2), NULLIF("v", $3), COALESCE("v", $4)`, nilValue, &fallback, nilValue, "unknown")

	calls := 0
	valuer := templateValuer{calls: &calls}
	custom := NewTypedColumn[templateValuer]("v")
	nullable := sql.NullString{String: "value", Valid: true}
	named := sql.Named("fallback", "value")
	q := Select().SelectExpr(custom.Coalesce(valuer), custom.NullIf(valuer),
		NewTypedColumn[sql.NullString]("v").Coalesce(nullable),
		NewTypedColumn[any]("v").NullIf(nil),
		NewTypedColumn[sql.NamedArg]("v").Coalesce(named), c.NullIfExpr(Value(named))).SetDialect(dialect.SQLite)
	want := []any{valuer, valuer, nullable, nil, named}
	checkSQL(t, q, `SELECT COALESCE("v", ?), NULLIF("v", ?), COALESCE("v", ?), NULLIF("v", ?), COALESCE("v", @fallback), NULLIF("v", @fallback)`, want...)
	_, args, err := mustCompileTemplate(t, q).Bind()
	if err != nil || calls != 0 || !reflect.DeepEqual(args, want) {
		t.Fatal("function data must bind unchanged without invoking driver conversion", args, err, calls)
	}

	dynamic := NewTypedColumn[any]("v")
	for _, expr := range []Expression{Expr("trusted_sql()"), Default()} {
		q := Select().SelectExpr(dynamic.Coalesce(expr), dynamic.NullIf(expr),
			NewTypedColumn[Expression]("v").Coalesce(expr), NewTypedColumn[Expression]("v").NullIf(expr)).SetDialect(dialect.Postgres)
		checkSQL(t, q, `SELECT COALESCE("v", $1), NULLIF("v", $2), COALESCE("v", $3), NULLIF("v", $4)`, expr, expr, expr, expr)
		_, args, err := mustCompileTemplate(t, q).Bind()
		if err != nil || !reflect.DeepEqual(args, []any{expr, expr, expr, expr}) {
			t.Fatal("ordinary expressions in data methods must remain data", args, err)
		}
	}
}

func TestTypedColumnFunctionTemplatesAndErrors(t *testing.T) {
	v := NewTypedColumn[int64]("v")
	dynamic := NewTypedColumn[any]("v")
	q := Select().SelectExpr(v.CoalesceExpr(Param(0), Value(21)), v.NullIfExpr(Param(1)),
		dynamic.Coalesce(Param(2)), dynamic.NullIf(Param(2))).SetDialect(dialect.Postgres)
	checkBuildError(t, q)
	compiled := mustCompileTemplate(t, q)
	for _, values := range [][]any{{nil, 1, "fallback"}, {7, nil, nil}} {
		query, args, err := compiled.Bind(values...)
		if err != nil || query != `SELECT COALESCE("v", $1, $2), NULLIF("v", $3), COALESCE("v", $4), NULLIF("v", $5)` ||
			!reflect.DeepEqual(args, []any{values[0], 21, values[1], values[2], values[2]}) {
			t.Fatal(query, args, err)
		}
	}
	for _, expr := range []Expression{
		v.Coalesce(), v.Coalesce([]int64(nil)...), v.Coalesce([]int64{}...),
		v.CoalesceExpr(), v.CoalesceExpr([]Expression(nil)...), v.CoalesceExpr([]Expression{}...),
		v.Cast(""), v.Cast(" \t "),
		v.CoalesceExpr(Default()), v.NullIfExpr(Default()),
		v.CoalesceExpr(Expr("? + ?", 1)), v.NullIfExpr(Expr("? + ?", 1)),
	} {
		q := Select().SelectExpr(expr)
		checkBuildError(t, q)
		if _, err := q.Compile(); err == nil {
			t.Fatal("invalid function expression compiled successfully")
		}
	}
}
