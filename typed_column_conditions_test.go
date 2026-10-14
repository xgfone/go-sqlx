// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"database/sql"
	"reflect"
	"testing"
	"time"

	"github.com/xgfone/go-sqlx/dialect"
)

func TestTypedColumnOrderedConditions(t *testing.T) {
	age := NewTypedColumn[int64]("age")
	for _, tc := range []struct {
		d    Dialect
		want string
	}{
		{dialect.MySQL, "SELECT `age` FROM `users` WHERE ((`age` > ?) AND (`age` >= ?) AND (`age` < ?) AND (`age` <= ?) AND (`age` BETWEEN ? AND ?) AND (NOT ((`age` BETWEEN ? AND ?))))"},
		{dialect.Postgres, `SELECT "age" FROM "users" WHERE (("age" > $1) AND ("age" >= $2) AND ("age" < $3) AND ("age" <= $4) AND ("age" BETWEEN $5 AND $6) AND (NOT (("age" BETWEEN $7 AND $8))))`},
		{dialect.SQLite, `SELECT "age" FROM "users" WHERE (("age" > ?) AND ("age" >= ?) AND ("age" < ?) AND ("age" <= ?) AND ("age" BETWEEN ? AND ?) AND (NOT (("age" BETWEEN ? AND ?))))`},
	} {
		t.Run(tc.d.Name(), func(t *testing.T) {
			checkSQL(t, Select("age").From("users").Where(
				age.Gt(17), age.Ge(18), age.Lt(66), age.Le(65),
				age.Between(18, 65), age.NotBetween(30, 40),
			).SetDialect(tc.d), tc.want,
				int64(17), int64(18), int64(66), int64(65),
				int64(18), int64(65), int64(30), int64(40),
			)
		})
	}
}

func TestTypedColumnListSnapshots(t *testing.T) {
	id := NewTypedColumn[int64]("id")
	values := []int64{1, 2}
	conditions := []Condition{id.In(values...), id.NotIn(values...), id.InSlice(values), id.NotInSlice(values)}
	clear(values)
	for _, tc := range []struct {
		d    Dialect
		want string
	}{
		{dialect.MySQL, "SELECT `id` FROM `t` WHERE ((`id` IN (?, ?)) AND (`id` NOT IN (?, ?)) AND (`id` IN (?, ?)) AND (`id` NOT IN (?, ?)))"},
		{dialect.Postgres, `SELECT "id" FROM "t" WHERE (("id" IN ($1, $2)) AND ("id" NOT IN ($3, $4)) AND ("id" IN ($5, $6)) AND ("id" NOT IN ($7, $8)))`},
		{dialect.SQLite, `SELECT "id" FROM "t" WHERE (("id" IN (?, ?)) AND ("id" NOT IN (?, ?)) AND ("id" IN (?, ?)) AND ("id" NOT IN (?, ?)))`},
	} {
		t.Run(tc.d.Name(), func(t *testing.T) {
			q := Select("id").From("t").Where(conditions...).SetDialect(tc.d)
			for range 2 {
				checkSQL(t, q, tc.want,
					int64(1), int64(2), int64(1), int64(2),
					int64(1), int64(2), int64(1), int64(2),
				)
			}
		})
	}

	value := int64(1)
	pointers := []*int64{&value}
	condition := NewTypedColumn[*int64]("id").InSlice(pointers)
	pointers[0], value = nil, 2
	checkSQL(t,
		Select("id").From("t").Where(condition).SetDialect(dialect.Postgres),
		`SELECT "id" FROM "t" WHERE ("id" IN ($1))`, &value)
}

func TestTypedColumnEmptyLists(t *testing.T) {
	id := NewTypedColumn[int64]("id")
	for _, condition := range []Condition{
		id.In(), id.In([]int64(nil)...), id.InSlice(nil), id.InSlice([]int64{}),
		id.InExpr(), id.InExpr([]Expression(nil)...), id.InExpr([]Expression{}...),
	} {
		checkSQL(t,
			Select("id").From("t").Where(condition).SetDialect(dialect.Postgres),
			`SELECT "id" FROM "t" WHERE (1=0)`)
	}
	for _, condition := range []Condition{
		id.NotIn(),
		id.NotIn([]int64(nil)...),
		id.NotInSlice(nil),
		id.NotInSlice([]int64{}),
		id.NotInExpr(),
		id.NotInExpr([]Expression(nil)...),
		id.NotInExpr([]Expression{}...),
	} {
		checkSQL(t,
			Select("id").From("t").Where(condition).SetDialect(dialect.Postgres),
			`SELECT "id" FROM "t" WHERE (1=1)`)
	}
}

func TestTypedColumnConditionNulls(t *testing.T) {
	age := NewTypedColumn[*int64]("age")
	dynamic := NewTypedColumn[any]("age")
	var nilValue *int64
	for _, tc := range []struct {
		name string
		c    Condition
		want string
		args []any
	}{
		{"greater", age.Gt(nil), `("age" > $1)`, []any{nilValue}},
		{"greater equal", age.Ge(nil), `("age" >= $1)`, []any{nilValue}},
		{"less", age.Lt(nil), `("age" < $1)`, []any{nilValue}},
		{"less equal", age.Le(nil), `("age" <= $1)`, []any{nilValue}},
		{"between", age.Between(nil, nil), `("age" BETWEEN $1 AND $2)`, []any{nilValue, nilValue}},
		{"not between", age.NotBetween(nil, nil), `(NOT (("age" BETWEEN $1 AND $2)))`, []any{nilValue, nilValue}},
		{"in", age.In(nil), `("age" IN ($1))`, []any{nilValue}},
		{"not in", age.NotIn(nil), `("age" NOT IN ($1))`, []any{nilValue}},
		{"in slice", age.InSlice([]*int64{nil}), `("age" IN ($1))`, []any{nilValue}},
		{"not in slice", age.NotInSlice([]*int64{nil}), `("age" NOT IN ($1))`, []any{nilValue}},
		{"nil interface", dynamic.Gt(nil), `("age" > $1)`, []any{nil}},
		{"mixed null list", dynamic.In(18, nil), `("age" IN ($1, $2))`, []any{18, nil}},
		{"mixed null negative list", dynamic.NotIn(18, nil), `("age" NOT IN ($1, $2))`, []any{18, nil}},
		{"explicit null range", age.BetweenExpr(Value(nil), Value(nil)), `("age" BETWEEN $1 AND $2)`, []any{nil, nil}},
		{"explicit null list", age.NotInExpr(Value(nil)), `("age" NOT IN ($1))`, []any{nil}},
		{"like nil pointer", age.Like(nil), `("age" LIKE $1)`, []any{nilValue}},
		{"not like nil pointer", age.NotLike(nil), `("age" NOT LIKE $1)`, []any{nilValue}},
		{"like nil interface", dynamic.Like(nil), `("age" LIKE $1)`, []any{nil}},
		{"not like nil interface", dynamic.NotLike(nil), `("age" NOT LIKE $1)`, []any{nil}},
		{"like explicit null", age.LikeExpr(Value(nil)), `("age" LIKE $1)`, []any{nil}},
		{"not like explicit null", age.NotLikeExpr(Value(nil)), `("age" NOT LIKE $1)`, []any{nil}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			checkSQL(t,
				Select("age").From("users").Where(tc.c).SetDialect(dialect.Postgres),
				`SELECT "age" FROM "users" WHERE `+tc.want, tc.args...)
		})
	}
}

func TestTypedColumnConditionDataTypes(t *testing.T) {
	type userID int64
	type userIDs []userID
	stamp := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	checkSQL(t, Select("id").From("users").Where(
		NewTypedColumn[userID]("id").Gt(7),
		NewTypedColumn[userID]("id").InSlice(userIDs{8, 9}),
		NewTypedColumn[string]("name").Between("A", "Z"),
		NewTypedColumn[time.Time]("created_at").Ge(stamp),
		NewTypedColumn[[]byte]("data").InSlice([][]byte{{1, 2}, {3, 4}}),
	).SetDialect(dialect.Postgres),
		`SELECT "id" FROM "users" WHERE (("id" > $1) AND ("id" IN ($2, $3)) AND ("name" BETWEEN $4 AND $5) AND ("created_at" >= $6) AND ("data" IN ($7, $8)))`,
		userID(7), userID(8), userID(9), "A", "Z", stamp, []byte{1, 2}, []byte{3, 4})

	calls := 0
	valuer := templateValuer{calls: &calls}
	v := NewTypedColumn[templateValuer]("v")
	nullable := sql.NullInt64{}
	q := Select("v").From("t").Where(
		v.Le(valuer), v.NotBetween(valuer, valuer), v.InSlice([]templateValuer{valuer}),
		NewTypedColumn[sql.NullInt64]("v").In(nullable),
	).SetDialect(dialect.Postgres)
	want := []any{valuer, valuer, valuer, valuer, nullable}
	checkSQL(t, q,
		`SELECT "v" FROM "t" WHERE (("v" <= $1) AND (NOT (("v" BETWEEN $2 AND $3))) AND ("v" IN ($4)) AND ("v" IN ($5)))`,
		want...)

	_, args, err := mustCompileTemplate(t, q).Bind()
	if err != nil || calls != 0 || !reflect.DeepEqual(args, want) {
		t.Fatal("condition data must bind unchanged without driver conversion", args, err, calls)
	}

	named := NewTypedColumn[sql.NamedArg]("v")
	checkSQL(t,
		Select("v").From("t").Where(
			named.Ge(sql.Named("minimum", 18)),
			named.In(sql.Named("minimum", 18)),
		).SetDialect(dialect.SQLite),
		`SELECT "v" FROM "t" WHERE (("v" >= @minimum) AND ("v" IN (@minimum)))`,
		sql.Named("minimum", 18))
}

func TestTypedColumnConditionDataDoesNotRenderExpressions(t *testing.T) {
	c := NewTypedColumn[any]("v")
	expr := Expr("trusted_sql()")
	for _, tc := range []struct {
		c    Condition
		want string
		args []any
	}{
		{NewTypedColumn[Expression]("v").In(expr), `("v" IN ($1))`, []any{expr}},
		{c.NotLike(Default()), `("v" NOT LIKE $1)`, []any{Default()}},
		{NewTypedColumn[Expression]("v").Like(expr), `("v" LIKE $1)`, []any{expr}},
		{NewTypedColumn[Expression]("v").NotLike(expr), `("v" NOT LIKE $1)`, []any{expr}},
	} {
		q := Select("v").From("t").Where(tc.c).SetDialect(dialect.Postgres)
		checkSQL(t, q, `SELECT "v" FROM "t" WHERE `+tc.want, tc.args...)
		_, args, err := mustCompileTemplate(t, q).Bind()
		if err != nil || !reflect.DeepEqual(args, tc.args) {
			t.Fatal("ordinary expressions in data methods must remain data", args, err)
		}
	}
}

func TestTypedColumnConditionExpressions(t *testing.T) {
	age := NewTypedColumn[int64]("age")
	for _, tc := range []struct {
		name string
		c    Condition
		want string
		args []any
	}{
		{"greater", age.GtExpr(Param(0)), `("age" > $1)`, []any{20}},
		{"greater equal", age.GeExpr(Expr("? + ?", Ident("minimum"), Param(0))), `("age" >= "minimum" + $1)`, []any{20}},
		{"less", age.LtExpr(Param(0)), `("age" < $1)`, []any{20}},
		{"less equal", age.LeExpr(Param(0)), `("age" <= $1)`, []any{20}},
		{"between", age.BetweenExpr(Value(int64(18)), Param(0)), `("age" BETWEEN $1 AND $2)`, []any{int64(18), 20}},
		{"not between", age.NotBetweenExpr(Param(0), Value(int64(65))), `(NOT (("age" BETWEEN $1 AND $2)))`, []any{20, int64(65)}},
		{"in", age.InExpr(Value(int64(18)), Param(0), Ident("minimum")), `("age" IN ($1, $2, "minimum"))`, []any{int64(18), 20}},
		{"not in", age.NotInExpr(Param(0), Value(nil)), `("age" NOT IN ($1, $2))`, []any{20, nil}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			q := Select("age").From("users").Where(tc.c).SetDialect(dialect.Postgres)
			checkBuildError(t, q) // Param still requires Compile.
			query, args, err := mustCompileTemplate(t, q).Bind(20)
			if err != nil || query != `SELECT "age" FROM "users" WHERE `+tc.want || !reflect.DeepEqual(args, tc.args) {
				t.Fatal(query, args, err)
			}
		})
	}

	exprs := []Expression{Value(18), NewTypedColumn[int64]("minimum").Ref()}
	q := Select("age").From("users").
		Where(age.InExpr(exprs...), age.NotInExpr(exprs...)).
		SetDialect(dialect.Postgres)
	clear(exprs)
	checkSQL(t, q, `SELECT "age" FROM "users" WHERE (("age" IN ($1, "minimum")) AND ("age" NOT IN ($2, "minimum")))`, 18, 18)

	values := []int64{18, 20}
	query, args, err := mustCompileTemplate(t,
		Select("age").From("users").Where(age.InExpr(Param(0))).SetDialect(dialect.Postgres),
	).Bind(values)
	if err != nil ||
		query != `SELECT "age" FROM "users" WHERE ("age" IN ($1))` ||
		!reflect.DeepEqual(args, []any{values}) {
		t.Fatal("a template slot must remain one bound value without slice expansion", query, args, err)
	}
}

func TestTypedColumnConditionExpressionErrors(t *testing.T) {
	c := NewTypedColumn[int64]("v")
	for _, expr := range []Expression{Default(), Expr("? + ?", 1)} {
		for _, condition := range []Condition{
			c.GtExpr(expr), c.GeExpr(expr), c.LtExpr(expr), c.LeExpr(expr),
			c.BetweenExpr(expr, Value(2)), c.BetweenExpr(Value(1), expr),
			c.NotBetweenExpr(expr, Value(2)), c.NotBetweenExpr(Value(1), expr),
			c.InExpr(expr), c.NotInExpr(expr),
			c.LikeExpr(expr), c.NotLikeExpr(expr),
		} {
			q := Select("v").From("t").Where(condition)
			checkBuildError(t, q)
			if _, err := q.Compile(); err == nil {
				t.Fatal("invalid condition expression compiled successfully")
			}
		}
	}
}

func TestTypedColumnQueryConditions(t *testing.T) {
	id := NewTypedColumn[int64]("id").Scope("u")
	subquery := Select("id").From("other").
		Where(Gt("id", Param(0))).
		SetDialect(dialect.MySQL)
	q := Select("u.id").FromAlias("t", "u").
		Where(id.InQuery(subquery), id.NotInQuery(subquery)).
		SetDialect(dialect.Postgres)

	subquery.Select("extra").Where(Eq("id", 999))
	query, args, err := mustCompileTemplate(t, q).Bind(7)
	want := `SELECT "u"."id" FROM "t" AS "u" WHERE ("u"."id" IN (SELECT "id" FROM "other" WHERE ("id" > $1)) AND "u"."id" NOT IN (SELECT "id" FROM "other" WHERE ("id" > $2)))`
	if err != nil || query != want || !reflect.DeepEqual(args, []any{7, 7}) {
		t.Fatal("subqueries must be snapshotted and use the parent binding context", query, args, err)
	}

	for _, condition := range []Condition{id.InQuery(nil), id.NotInQuery(nil)} {
		b := Select("id").FromAlias("t", "u").Where(condition)
		checkBuildError(t, b)
		if _, err := b.Compile(); err == nil {
			t.Fatal("nil subquery compiled successfully")
		}
	}
}

func TestNumericColumnConditions(t *testing.T) {
	v := NumericColumn[int64]("v").Scope("t")
	checkSQL(t,
		Select("t.v").From("t").Where(
			v.Gt(0), v.Between(1, 3), v.InSlice([]int64{1, 2}), v.LeExpr(Value(3)),
		).SetDialect(dialect.Postgres),
		`SELECT "t"."v" FROM "t" WHERE (("t"."v" > $1) AND ("t"."v" BETWEEN $2 AND $3) AND ("t"."v" IN ($4, $5)) AND ("t"."v" <= $6))`,
		int64(0), int64(1), int64(3), int64(1), int64(2), 3)
}
