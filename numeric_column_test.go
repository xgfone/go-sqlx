// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"database/sql"
	"reflect"
	"testing"

	"github.com/xgfone/go-sqlx/dialect"
)

func TestNewNumericColumnNames(t *testing.T) {
	type fieldName string
	name := "score"
	column := Column(name)
	custom := fieldName(name)
	fromString := NewNumericColumn[int64, string]
	for _, tc := range []struct {
		name   string
		column NumericColumn[int64]
	}{
		{"literal", NewNumericColumn[int64]("score")},
		{"string variable", NewNumericColumn[int64](name)},
		{"Column variable", NewNumericColumn[int64](column)},
		{"defined string variable", NewNumericColumn[int64](custom)},
		{"function value", fromString(name)},
		{"typed column", Numeric(column.Typed[int64]())},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.column.Name() != name || tc.column.Column() != column ||
				tc.column.TypedColumn != column.Typed[int64]() {
				t.Fatal("constructor changed the column path or type", tc.column)
			}
			checkSQL(t, Update().Table("users").Set(tc.column.Add(2)).
				SetDialect(dialect.Postgres), `UPDATE "users" SET "score"="score" + $1`, int64(2))
		})
	}
}

func TestNumericColumnScopeAndTypedOperations(t *testing.T) {
	age := NewNumericColumn[int64]("age")
	scoped := age.Scope("u").Scope("app")
	if scoped.Name() != "app.u.age" || age.Name() != "age" || age.Scope("") != age {
		t.Fatal("Scope must preserve the numeric column type and the original path")
	}

	quoted := NewNumericColumn[int64](Column(`a"b`)).Scope(`u"x`)
	checkSQL(t,
		Select().SelectExpr(quoted.Ref()).SetDialect(dialect.Postgres),
		`SELECT "u""x"."a""b"`)
	checkSQL(t,
		Update().Table("app.u").Set(age.AddExpr(scoped.Ref())).
			SetDialect(dialect.Postgres),
		`UPDATE "app"."u" SET "age"="age" + ("app"."u"."age")`)
	checkSQL(t,
		SelectColumns(scoped.Column()).From("app.u").Where(scoped.Eq(18)).
			SetDialect(dialect.Postgres),
		`SELECT "app"."u"."age" FROM "app"."u" WHERE ("app"."u"."age" = $1)`, int64(18))
	checkSQL(t,
		Update().TableAlias("users", `u"x`).Set(quoted.Inc()).SetDialect(dialect.MySQL),
		"UPDATE `users` AS `u\"x` SET `u\"x`.`a\"b`=`u\"x`.`a\"b` + 1")

	var zero NumericColumn[int]
	if zero.Name() != "" || zero.Column() != "" || zero.TypedColumn != (TypedColumn[int]{}) {
		t.Fatal("zero numeric column must retain the zero typed column")
	}

	backup := NewTypedColumn[int64]("backup_age")
	id := NewNumericColumn[int64]("id")
	checkSQL(t,
		Update().Table("users").Set(age.SetNamed("age", 18), backup.SetFrom(age.TypedColumn)).
			Where(id.Eq(7), age.Ne(0)).SetDialect(dialect.SQLite),
		`UPDATE "users" SET "age"=@age, "backup_age"="age" WHERE (("id" = ?) AND ("age" <> ?))`,
		sql.Named("age", int64(18)), int64(7), int64(0))
	checkSQL(t,
		Insert().Into("users").Row(id.ColValue(7), age.ColValueExpr(Value(18))).
			SetDialect(dialect.Postgres),
		`INSERT INTO "users" ("id", "age") VALUES ($1, $2)`,
		int64(7), 18)
	checkSQL(t,
		Update().Table("users").Set(age.Set(18), backup.SetFrom(age.TypedColumn)).
			SetDialect(dialect.Postgres),
		`UPDATE "users" SET "age"=$1, "backup_age"="age"`,
		int64(18))
	checkSQL(t,
		Update().Table("users").Set(age.SetExpr(Default())).
			SetDialect(dialect.Postgres),
		`UPDATE "users" SET "age"=DEFAULT`)
}

func TestNumericColumnArithmetic(t *testing.T) {
	type count uint32
	score := NewNumericColumn[int64]("score")
	balance := Numeric(Column("balance").Typed[float64]())
	factor := NewNumericColumn[count]("factor")
	ratio := NewNumericColumn[float32]("ratio")
	visits := NewNumericColumn[uint64]("visits")
	remaining := NewNumericColumn[int]("remaining")
	for _, tc := range []struct {
		d    Dialect
		want string
	}{
		{dialect.MySQL, "UPDATE `t` SET `score`=`score` + ?, `balance`=`balance` - ?, `factor`=`factor` * ?, `ratio`=`ratio` / ?, `visits`=`visits` + 1, `remaining`=`remaining` - 1 WHERE (`id` = ?)"},
		{dialect.Postgres, `UPDATE "t" SET "score"="score" + $1, "balance"="balance" - $2, "factor"="factor" * $3, "ratio"="ratio" / $4, "visits"="visits" + 1, "remaining"="remaining" - 1 WHERE ("id" = $5)`},
		{dialect.SQLite, `UPDATE "t" SET "score"="score" + ?, "balance"="balance" - ?, "factor"="factor" * ?, "ratio"="ratio" / ?, "visits"="visits" + 1, "remaining"="remaining" - 1 WHERE ("id" = ?)`},
	} {
		t.Run(tc.d.Name(), func(t *testing.T) {
			checkSQL(t,
				Update().Table("t").
					Set(Batch(
						score.Add(-2), balance.Sub(2.5),
						factor.Mul(3), ratio.Div(2),
						visits.Inc(), remaining.Dec(),
					)).
					Where(NewTypedColumn[int64]("id").Eq(7)).
					SetDialect(tc.d),
				tc.want,
				int64(-2), float64(2.5), count(3), float32(2), int64(7))
		})
	}
}

func TestNumericColumnNumberTypes(t *testing.T) {
	checkNumericColumnNumber(t, int(2))
	checkNumericColumnNumber(t, int8(2))
	checkNumericColumnNumber(t, int16(2))
	checkNumericColumnNumber(t, int32(2))
	checkNumericColumnNumber(t, int64(2))
	checkNumericColumnNumber(t, uint(2))
	checkNumericColumnNumber(t, uint8(2))
	checkNumericColumnNumber(t, uint16(2))
	checkNumericColumnNumber(t, uint32(2))
	checkNumericColumnNumber(t, uint64(2))
	checkNumericColumnNumber(t, float32(2))
	checkNumericColumnNumber(t, float64(2))

	type count int64
	type ratio float64
	checkNumericColumnNumber(t, count(2))
	checkNumericColumnNumber(t, ratio(2))
}

func checkNumericColumnNumber[T Number](t *testing.T, value T) {
	t.Helper()
	t.Run(reflect.TypeFor[T]().String(), func(t *testing.T) {
		checkSQL(t,
			Update().Table("t").Set(
				NewNumericColumn[T]("a").Add(value),
				Numeric(NewTypedColumn[T]("b")).Sub(value),
				NewNumericColumn[T]("c").Mul(value),
				Numeric(NewTypedColumn[T]("d")).Div(value),
			).SetDialect(dialect.Postgres),
			`UPDATE "t" SET "a"="a" + $1, "b"="b" - $2, "c"="c" * $3, "d"="d" / $4`,
			value, value, value, value)
	})
}

func TestNumericColumnExpressionOperands(t *testing.T) {
	bonus := NewTypedColumn[int64]("bonus")
	for _, tc := range []struct {
		name string
		u    Updater
		want string
		args []any
	}{
		{"add", NewNumericColumn[int64]("v").AddExpr(Expr("? - ?", bonus.Ref(), 2)), `"v"="v" + ("bonus" - $1)`, []any{2}},
		{"sub", NewNumericColumn[int64]("v").SubExpr(Expr("? + ?", bonus.Ref(), 2)), `"v"="v" - ("bonus" + $1)`, []any{2}},
		{"mul", NewNumericColumn[int64]("v").MulExpr(Expr("? + ?", bonus.Ref(), 2)), `"v"="v" * ("bonus" + $1)`, []any{2}},
		{"div", NewNumericColumn[int64]("v").DivExpr(Expr("? * ?", bonus.Ref(), 2)), `"v"="v" / ("bonus" * $1)`, []any{2}},
		{"column reference", NewNumericColumn[int64]("v").AddExpr(bonus.Scope("other").Ref()), `"v"="v" + ("other"."bonus")`, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			checkSQL(t, Update().Table("t").Set(tc.u).SetDialect(dialect.Postgres),
				`UPDATE "t" SET `+tc.want, tc.args...)
		})
	}

	value := NewNumericColumn[int64]("v")
	for _, method := range []func(Expression) Updater{
		value.AddExpr, value.SubExpr, value.MulExpr, value.DivExpr,
	} {
		for _, expr := range []Expression{Default(), Expr("? + ?", 1)} {
			b := Update().Table("t").Set(method(expr))
			checkBuildError(t, b)
			if _, err := b.Compile(); err == nil {
				t.Fatal("invalid arithmetic expression compiled successfully")
			}
		}
	}
}

func TestNumericColumnExplicitValues(t *testing.T) {
	calls := 0
	valuer := templateValuer{calls: &calls}
	value := int64(3)
	nullable := sql.NullInt64{Int64: 4, Valid: true}
	b := Update().Table("t").Set(
		NewNumericColumn[int64]("a").AddExpr(Value(valuer)),
		NewNumericColumn[int64]("b").SubExpr(Value(&value)),
		NewNumericColumn[int64]("c").MulExpr(Value(nullable)),
		NewNumericColumn[int64]("d").DivExpr(Value(nil)),
	).SetDialect(dialect.Postgres)

	want := []any{valuer, &value, nullable, nil}
	checkSQL(t, b, `UPDATE "t" SET "a"="a" + ($1), "b"="b" - ($2), "c"="c" * ($3), "d"="d" / ($4)`, want...)
	_, args, err := mustCompileTemplate(t, b).Bind()
	if err != nil || calls != 0 || !reflect.DeepEqual(args, want) {
		t.Fatal("explicit values must bind unchanged without driver conversion", args, err, calls)
	}

	checkSQL(t, Update().Table("t").Set(
		NewNumericColumn[int64]("a").AddExpr(Value(sql.Named("delta", 2))),
		NewNumericColumn[int64]("b").SubExpr(Value(sql.Named("delta", 2))),
	).SetDialect(dialect.SQLite),
		`UPDATE "t" SET "a"="a" + (@delta), "b"="b" - (@delta)`, sql.Named("delta", 2))
}

func TestNumericColumnTemplatesAndUpserts(t *testing.T) {
	q := mustCompileTemplate(t, Update().Table("t").
		Set(
			NewNumericColumn[int64]("a").AddExpr(Param(0)),
			NewNumericColumn[int64]("b").SubExpr(Param(1)),
			NewNumericColumn[int64]("c").MulExpr(Param(2)),
			NewNumericColumn[int64]("d").DivExpr(Param(3)),
			NewNumericColumn[int64]("e").Add(2),
			NewNumericColumn[int64]("f").Inc(),
			NewNumericColumn[int64]("g").Dec(),
		).
		Where(NewNumericColumn[int64]("id").EqExpr(Param(4))).
		SetDialect(dialect.Postgres))

	for _, values := range [][]any{{10, 2, 3, 2, 7}, {-3, 0, 4, 5, 8}} {
		query, args, err := q.Bind(values...)
		wantSQL := `UPDATE "t" SET "a"="a" + $1, "b"="b" - $2, "c"="c" * $3, "d"="d" / $4, "e"="e" + $5, "f"="f" + 1, "g"="g" - 1 WHERE ("id" = $6)`
		wantArgs := append(append([]any(nil), values[:4]...), int64(2), values[4])
		if err != nil || query != wantSQL || !reflect.DeepEqual(args, wantArgs) {
			t.Fatal(query, args, err)
		}
	}

	v := NewNumericColumn[int64]("v")
	checkSQL(t,
		Insert().Into("t").Columns("id", "v").Values(1, 2).
			OnConflict(ConflictColumns("id").DoUpdate(v.AddExpr(Excluded("v")))).
			SetDialect(dialect.Postgres),
		`INSERT INTO "t" ("id", "v") VALUES ($1, $2) ON CONFLICT ("id") DO UPDATE SET "v"="v" + ("excluded"."v")`,
		1, 2)
	checkSQL(t,
		Insert().Into("t").Columns("id", "v").Values(1, 2).
			OnDuplicateKeyUpdate(v.Inc()).
			SetDialect(dialect.MySQL),
		"INSERT INTO `t` (`id`, `v`) VALUES (?, ?) ON DUPLICATE KEY UPDATE `v`=`v` + 1",
		1, 2)
}
