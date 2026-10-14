// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
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
		column TypedColumn[int64]
	}{
		{"literal", NewNumericColumn[int64]("score")},
		{"string variable", NewNumericColumn[int64](name)},
		{"Column variable", NewNumericColumn[int64](column)},
		{"defined string variable", NewNumericColumn[int64](custom)},
		{"function value", fromString(name)},
		{"projected typed column", NewNumericColumn[int64](column.Typed[int64]().Column())},
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

func TestNewNumericColumnNumberTypes(t *testing.T) {
	checkNewNumericColumnNumber(t, int(2))
	checkNewNumericColumnNumber(t, int8(2))
	checkNewNumericColumnNumber(t, int16(2))
	checkNewNumericColumnNumber(t, int32(2))
	checkNewNumericColumnNumber(t, int64(2))
	checkNewNumericColumnNumber(t, uint(2))
	checkNewNumericColumnNumber(t, uint8(2))
	checkNewNumericColumnNumber(t, uint16(2))
	checkNewNumericColumnNumber(t, uint32(2))
	checkNewNumericColumnNumber(t, uint64(2))
	checkNewNumericColumnNumber(t, float32(2))
	checkNewNumericColumnNumber(t, float64(2))

	type count int64
	type ratio float64
	checkNewNumericColumnNumber(t, count(2))
	checkNewNumericColumnNumber(t, ratio(2))
}

func checkNewNumericColumnNumber[T Number](t *testing.T, value T) {
	t.Helper()
	t.Run(reflect.TypeFor[T]().String(), func(t *testing.T) {
		checkSQL(t,
			Update().Table("t").Set(
				NewNumericColumn[T]("a").Add(value),
				NewNumericColumn[T]("b").Sub(value),
				NewNumericColumn[T]("c").Mul(value),
				NewNumericColumn[T]("d").Div(value),
			).SetDialect(dialect.Postgres),
			`UPDATE "t" SET "a"="a" + $1, "b"="b" - $2, "c"="c" * $3, "d"="d" / $4`,
			value, value, value, value)
	})
}
