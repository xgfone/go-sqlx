// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
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
