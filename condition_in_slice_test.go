// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"reflect"
	"testing"

	"github.com/xgfone/go-sqlx/dialect"
)

func TestInSliceTypedValues(t *testing.T) {
	testInSliceValues(t, "int64", []int64{1, 2}, int64(1), int64(2))
	testInSliceValues(t, "uint64", []uint64{1, 1 << 63}, uint64(1), uint64(1<<63))
	testInSliceValues(t, "int", []int{1, 2}, 1, 2)
	testInSliceValues(t, "uint", []uint{1, 2}, uint(1), uint(2))
	testInSliceValues(t, "string", []string{"one", "two"}, "one", "two")
	testInSliceValues(t, "any_with_null", []any{1, nil}, 1, nil)

	type userID int64
	type userIDs []userID
	testInSliceValues(t, "defined_element_and_slice", userIDs{1, 2}, userID(1), userID(2))
}

func testInSliceValues[V any](t *testing.T, name string, values []V, args ...any) {
	t.Helper()
	t.Run(name, func(t *testing.T) {
		type path string
		const id Column = "t.id"
		conditions := []struct {
			name  string
			in    Condition
			notIn Condition
		}{
			{"string", InSlice("t.id", values), NotInSlice("t.id", values)},
			{"defined_string", InSlice(path("t.id"), values), NotInSlice(path("t.id"), values)},
			{"expression", InSlice(Ident("t", "id"), values), NotInSlice(Ident("t", "id"), values)},
			{"column", id.InSlice(values), id.NotInSlice(values)},
		}

		// Conditions own the slice container, including when values is []any.
		clear(values)
		wantArgs := append(append([]any(nil), args...), args...)
		for _, c := range conditions {
			t.Run(c.name, func(t *testing.T) {
				q := Select("id").Where(c.in, c.notIn).SetDialect(dialect.Postgres)
				const want = `SELECT "id" WHERE (("t"."id" IN ($1, $2)) AND ("t"."id" NOT IN ($3, $4)))`
				checkSQL(t, q, want, wantArgs...)
				checkSQL(t, q, want, wantArgs...)
			})
		}
	})
}

func TestInSliceEmpty(t *testing.T) {
	const id Column = "id"
	for _, c := range []Condition{
		InSlice("id", []int64(nil)),
		InSlice("id", []string{}),
		id.InSlice([]uint64(nil)),
		id.InSlice([]uint{}),
	} {
		checkSQL(t, Select("id").Where(c).SetDialect(dialect.Postgres), `SELECT "id" WHERE (1=0)`)
	}

	for _, c := range []Condition{
		NotInSlice("id", []int64(nil)),
		NotInSlice("id", []string{}),
		id.NotInSlice([]uint64(nil)),
		id.NotInSlice([]uint{}),
	} {
		checkSQL(t, Select("id").Where(c).SetDialect(dialect.Postgres), `SELECT "id" WHERE (1=1)`)
	}
}

func TestInSliceExpressionsAndTemplate(t *testing.T) {
	values := []Expression{Ident("other"), Param(0), Expr("? + ?", 1, 2)}
	q := Select("id").Where(
		InSlice("id", values),
		Column("id").NotInSlice([]Expression{Value(nil), Param(1)}),
	).SetDialect(dialect.Postgres)
	clear(values)
	template := mustCompileTemplate(t, q)
	sql, args, err := template.Bind(10, 20)
	const want = `SELECT "id" WHERE (("id" IN ("other", $1, $2 + $3)) AND ("id" NOT IN ($4, $5)))`
	if err != nil || sql != want || !reflect.DeepEqual(args, []any{10, 1, 2, nil, 20}) {
		t.Fatal(sql, args, err)
	}
}

func TestInSliceTuples(t *testing.T) {
	row := Tuple(Ident("a"), Ident("b"))
	values := []Expression{Tuple(1, 2), Tuple(3, 4)}
	for _, tc := range []struct {
		dialect Dialect
		want    string
	}{
		{dialect.Postgres, `SELECT "a" WHERE (("a", "b") IN (($1, $2), ($3, $4)))`},
		{dialect.SQLite, `SELECT "a" WHERE (("a", "b") IN (VALUES (?, ?), (?, ?)))`},
		{dialect.MySQL, "SELECT `a` WHERE ((`a`, `b`) IN ((?, ?), (?, ?)))"},
	} {
		t.Run(tc.dialect.Name(), func(t *testing.T) {
			checkSQL(t, Select("a").Where(InSlice(row, values)).SetDialect(tc.dialect),
				tc.want, 1, 2, 3, 4)
		})
	}

	for _, c := range []Condition{
		InSlice(row, []Expression{Tuple(1, 2, 3)}),
		NotInSlice(row, []Expression{Tuple(1, 2, 3)}),
		InSlice(row, []int{1, 2}),
		NotInSlice(row, []int{1, 2}),
	} {
		checkBuildError(t, Select("a").Where(c))
	}
}
