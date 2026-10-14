// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"database/sql"
	"reflect"
	"testing"

	"github.com/xgfone/go-sqlx/dialect"
)

func TestNumericColumnNames(t *testing.T) {
	type fieldName string
	name := "score"
	column := Column(name)
	custom := fieldName(name)
	fromString := NumericColumn[int64, string]
	for _, tc := range []struct {
		name   string
		column TypedColumn[int64]
	}{
		{"literal", NumericColumn[int64]("score")},
		{"string variable", NumericColumn[int64](name)},
		{"Column variable", NumericColumn[int64](column)},
		{"defined string variable", NumericColumn[int64](custom)},
		{"function value", fromString(name)},
		{"projected typed column", NumericColumn[int64](column.Typed[int64]().Column())},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.column.Name() != name || tc.column.Column() != column ||
				tc.column != column.Typed[int64]() {
				t.Fatal("constructor changed the column path or type", tc.column)
			}
			checkSQL(t, Update().Table("users").Set(tc.column.Add(2)).
				SetDialect(dialect.Postgres), `UPDATE "users" SET "score"="score" + $1`, int64(2))
		})
	}
}

func TestNumericColumnScopeAndTypedOperations(t *testing.T) {
	age := NumericColumn[int64]("age")
	scoped := age.Scope("u").Scope("app")
	if scoped.Name() != "app.u.age" || age.Name() != "age" || age.Scope("") != age {
		t.Fatal("Scope must preserve T and the original path")
	}

	quoted := NumericColumn[int64](Column(`a"b`)).Scope(`u"x`)
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

	backup := NewTypedColumn[int64]("backup_age")
	id := NumericColumn[int64]("id")
	checkSQL(t,
		Update().Table("users").Set(age.SetNamed("age", 18), backup.SetFrom(age)).
			Where(id.Eq(7), age.Ne(0)).SetDialect(dialect.SQLite),
		`UPDATE "users" SET "age"=@age, "backup_age"="age" WHERE (("id" = ?) AND ("age" <> ?))`,
		sql.Named("age", int64(18)), int64(7), int64(0))
	checkSQL(t,
		Insert().Into("users").Row(id.ColValue(7), age.ColValueExpr(Value(18))).
			SetDialect(dialect.Postgres),
		`INSERT INTO "users" ("id", "age") VALUES ($1, $2)`,
		int64(7), 18)
	checkSQL(t,
		Update().Table("users").Set(age.Set(18), backup.SetFrom(age)).
			SetDialect(dialect.Postgres),
		`UPDATE "users" SET "age"=$1, "backup_age"="age"`,
		int64(18))
	checkSQL(t,
		Update().Table("users").Set(age.SetExpr(Default())).
			SetDialect(dialect.Postgres),
		`UPDATE "users" SET "age"=DEFAULT`)
}

func TestNumericColumnResult(t *testing.T) {
	var constructor = NumericColumn[int64, string]
	numeric := constructor("age")
	typed := NewTypedColumn[int64]("age")
	if typed != numeric || reflect.TypeOf(numeric) != reflect.TypeFor[TypedColumn[int64]]() {
		t.Fatal("NumericColumn must return the same TypedColumn value as NewTypedColumn")
	}

	backup := NumericColumn[int64]("backup_age")
	checkSQL(t, Update().Table("users").Set(typed.SetFrom(backup), backup.SetFrom(numeric)).
		SetDialect(dialect.Postgres),
		`UPDATE "users" SET "age"="backup_age", "backup_age"="age"`)
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
				NumericColumn[T]("a").Add(value),
				NumericColumn[T]("b").Sub(value),
				NumericColumn[T]("c").Mul(value),
				NumericColumn[T]("d").Div(value),
			).SetDialect(dialect.Postgres),
			`UPDATE "t" SET "a"="a" + $1, "b"="b" - $2, "c"="c" * $3, "d"="d" / $4`,
			value, value, value, value)
	})
}
