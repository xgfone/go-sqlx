// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"database/sql"
	"reflect"
	"testing"

	"github.com/xgfone/go-sqlx/dialect"
)

func TestArithmeticUpdates(t *testing.T) {
	type columnName string
	const score Column = "score"
	const balance Column = "balance"
	const factor Column = "factor"
	const ratio Column = "ratio"
	const visits Column = "visits"
	const remaining Column = "remaining"
	for _, api := range []struct {
		name    string
		setters []Updater
	}{
		{"functions", []Updater{Add(columnName("score"), 10), Sub(balance, 2.5), Mul("factor", 3), Div(ratio, 2), Inc("visits"), Dec(remaining)}},
		{"methods", []Updater{score.Add(10), balance.Sub(2.5), factor.Mul(3), ratio.Div(2), visits.Inc(), remaining.Dec()}},
	} {
		for _, tc := range []struct {
			d    Dialect
			want string
		}{
			{dialect.MySQL, "UPDATE `t` SET `score`=`score` + ?, `balance`=`balance` - ?, `factor`=`factor` * ?, `ratio`=`ratio` / ?, `visits`=`visits` + 1, `remaining`=`remaining` - 1 WHERE (`id` = ?)"},
			{dialect.Postgres, `UPDATE "t" SET "score"="score" + $1, "balance"="balance" - $2, "factor"="factor" * $3, "ratio"="ratio" / $4, "visits"="visits" + 1, "remaining"="remaining" - 1 WHERE ("id" = $5)`},
			{dialect.SQLite, `UPDATE "t" SET "score"="score" + ?, "balance"="balance" - ?, "factor"="factor" * ?, "ratio"="ratio" / ?, "visits"="visits" + 1, "remaining"="remaining" - 1 WHERE ("id" = ?)`},
		} {
			t.Run(api.name+"/"+tc.d.Name(), func(t *testing.T) {
				checkSQL(t, Update().Table("t").Set(Batch(api.setters...)).Where(Eq("id", 7)).SetDialect(tc.d),
					tc.want, 10, 2.5, 3, 2, 7)
			})
		}
	}
}

func TestArithmeticUpdateOperands(t *testing.T) {
	for _, tc := range []struct {
		name string
		u    Updater
		want string
		args []any
	}{
		{"zero", Add("v", 0), `"v"="v" + $1`, []any{0}},
		{"negative", Sub("v", -3), `"v"="v" - $1`, []any{-3}},
		{"null", Add("v", Value(nil)), `"v"="v" + ($1)`, []any{nil}},
		{"string data", Add("v", Value("bonus'); DROP TABLE t; --")), `"v"="v" + ($1)`, []any{"bonus'); DROP TABLE t; --"}},
		{"column data", Sub("v", Value(Column("bonus"))), `"v"="v" - ($1)`, []any{Column("bonus")}},
		{"column reference", Add("v", Column("bonus").Ref()), `"v"="v" + ("bonus")`, nil},
		{"identifier", Sub("v", Ident("other", "bonus")), `"v"="v" - ("other"."bonus")`, nil},
		{"add expression", Add("v", Expr("? - ?", Ident("bonus"), 2)), `"v"="v" + ("bonus" - $1)`, []any{2}},
		{"sub expression", Sub("v", Expr("? + ?", Ident("bonus"), 2)), `"v"="v" - ("bonus" + $1)`, []any{2}},
		{"mul expression", Column("v").Mul(Expr("? + ?", Ident("bonus"), 2)), `"v"="v" * ("bonus" + $1)`, []any{2}},
		{"div expression", Column("v").Div(Expr("? * ?", Ident("bonus"), 2)), `"v"="v" / ("bonus" * $1)`, []any{2}},
		{"scoped quoted column", Column(`s"x`).Scope(`t"x`).Inc(), `"t""x"."s""x"="t""x"."s""x" + 1`, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			checkSQL(t, Update().Table("t").Set(tc.u).SetDialect(dialect.Postgres),
				`UPDATE "t" SET `+tc.want, tc.args...)
		})
	}

	checkSQL(t, Update().Table("t").Set(Set("first", sql.Named("delta", 2)),
		Add("a", Value(sql.Named("delta", 2))), Sub("b", Value(sql.Named("delta", 2))),
		Mul("c", Value(sql.Named("delta", 2))), Div("d", Value(sql.Named("delta", 2)))).SetDialect(dialect.SQLite),
		`UPDATE "t" SET "first"=@delta, "a"="a" + (@delta), "b"="b" - (@delta), "c"="c" * (@delta), "d"="d" / (@delta)`, sql.Named("delta", 2))
	for _, u := range []Updater{Add("v", Default()), Sub("v", Default()), Mul("v", Default()), Div("v", Default())} {
		checkBuildError(t, Update().Table("t").Set(u))
	}
}

func TestArithmeticUpdateTemplate(t *testing.T) {
	const balance Column = "balance"
	q := mustCompileTemplate(t, Update().Table("t").
		Set(Add("score", Param(0)), balance.Sub(Param(1)), Mul("factor", Param(2)), Column("ratio").Div(Param(3)), Inc("visits"), Dec("remaining")).
		Where(Eq("id", Param(4))).SetDialect(dialect.Postgres))
	for _, values := range [][]any{{10, 2, 3, 2, 7}, {-3, 0, 4, 5, 8}} {
		query, args, err := q.Bind(values...)
		want := `UPDATE "t" SET "score"="score" + $1, "balance"="balance" - $2, "factor"="factor" * $3, "ratio"="ratio" / $4, "visits"="visits" + 1, "remaining"="remaining" - 1 WHERE ("id" = $5)`
		if err != nil || query != want || !reflect.DeepEqual(args, values) {
			t.Fatalf("got %q %v %v, want %q %v", query, args, err, want, values)
		}
	}
}

func TestArithmeticNumberTypes(t *testing.T) {
	checkArithmeticNumber(t, int(2))
	checkArithmeticNumber(t, int8(2))
	checkArithmeticNumber(t, int16(2))
	checkArithmeticNumber(t, int32(2))
	checkArithmeticNumber(t, int64(2))
	checkArithmeticNumber(t, uint(2))
	checkArithmeticNumber(t, uint8(2))
	checkArithmeticNumber(t, uint16(2))
	checkArithmeticNumber(t, uint32(2))
	checkArithmeticNumber(t, uint64(2))
	checkArithmeticNumber(t, float32(2))
	checkArithmeticNumber(t, float64(2))
	type count int64
	type ratio float64
	checkArithmeticNumber(t, count(2))
	checkArithmeticNumber(t, ratio(2))
}

func checkArithmeticNumber[V Number](t *testing.T, value V) {
	t.Helper()
	t.Run(reflect.TypeFor[V]().String(), func(t *testing.T) {
		checkSQL(t, Update().Table("t").Set(
			Add("a", value), Sub("b", value), Mul("c", value), Div("d", value),
			Column("e").Add(value), Column("f").Sub(value), Column("g").Mul(value), Column("h").Div(value),
		).SetDialect(dialect.Postgres),
			`UPDATE "t" SET "a"="a" + $1, "b"="b" - $2, "c"="c" * $3, "d"="d" / $4, "e"="e" + $5, "f"="f" - $6, "g"="g" * $7, "h"="h" / $8`,
			value, value, value, value, value, value, value, value)
	})
}

func TestArithmeticWrappedValues(t *testing.T) {
	calls := 0
	valuer := templateValuer{calls: &calls}
	value := 3
	var dynamic any = 2
	nullable := sql.NullInt64{Int64: 4, Valid: true}
	q := Update().Table("t").Set(
		Column("a").Add(Value(valuer)), Column("b").Sub(Value(&value)),
		Column("c").Mul(Value(dynamic)), Column("d").Div(Value(nullable)),
	).SetDialect(dialect.Postgres)
	checkSQL(t, q,
		`UPDATE "t" SET "a"="a" + ($1), "b"="b" - ($2), "c"="c" * ($3), "d"="d" / ($4)`,
		valuer, &value, dynamic, nullable)
	tmpl := mustCompileTemplate(t, q)
	_, args, err := tmpl.Bind()
	if err != nil || calls != 0 || !reflect.DeepEqual(args, []any{valuer, &value, dynamic, nullable}) {
		t.Fatal("wrapped values must bind unchanged without invoking Valuer", args, err, calls)
	}
}

func TestBatchEmptyComposition(t *testing.T) {
	for _, empty := range []Updater{
		Batch(),
		Batch([]Updater(nil)...),
		Batch([]Updater{}...),
	} {
		if empty != nil {
			t.Fatal("empty batch must return nil")
		}
	}

	empty := Batch()
	checkSQL(t, Update().Table("t").Set(empty, Set("a", 1), empty),
		"UPDATE `t` SET `a`=?", 1)
	checkSQL(t, Update().Table("t").Set(Batch(empty, Set("a", 1), empty)),
		"UPDATE `t` SET `a`=?", 1)

	checkBuildError(t, Update().Table("t").Set(empty))
	checkBuildError(t, Update().Table("t").Set(Batch(nil)))
	checkBuildError(t, Insert().Into("t").Values(1).OnDuplicateKeyUpdate(empty))
	checkBuildError(t, Insert().Into("t").Values(1).
		OnConflict(ConflictColumns("id").DoUpdate(empty)).SetDialect(dialect.Postgres))
}
