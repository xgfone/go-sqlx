// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"database/sql"
	"reflect"
	"testing"

	"github.com/xgfone/go-sqlx/dialect"
)

func TestTypedColumnArithmetic(t *testing.T) {
	type count uint32
	score := NewTypedColumn[int64]("score")
	balance := Column("balance").Typed[float64]()
	factor := NewTypedColumn[count]("factor")
	ratio := NewTypedColumn[float32]("ratio")
	visits := NewTypedColumn[uint64]("visits")
	remaining := NewTypedColumn[int]("remaining")
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
					Set(CombineUpdaters(
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

func TestTypedColumnArithmeticExpressionOperands(t *testing.T) {
	bonus := NewTypedColumn[int64]("bonus")
	for _, tc := range []struct {
		name string
		u    Updater
		want string
		args []any
	}{
		{"add", NewTypedColumn[int64]("v").AddExpr(Expr("? - ?", bonus.Ref(), 2)), `"v"="v" + ("bonus" - $1)`, []any{2}},
		{"sub", NewTypedColumn[int64]("v").SubExpr(Expr("? + ?", bonus.Ref(), 2)), `"v"="v" - ("bonus" + $1)`, []any{2}},
		{"mul", NewTypedColumn[int64]("v").MulExpr(Expr("? + ?", bonus.Ref(), 2)), `"v"="v" * ("bonus" + $1)`, []any{2}},
		{"div", NewTypedColumn[int64]("v").DivExpr(Expr("? * ?", bonus.Ref(), 2)), `"v"="v" / ("bonus" * $1)`, []any{2}},
		{"column reference", NewTypedColumn[int64]("v").AddExpr(bonus.Qualify("other").Ref()), `"v"="v" + ("other"."bonus")`, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			checkSQL(t, Update().Table("t").Set(tc.u).SetDialect(dialect.Postgres),
				`UPDATE "t" SET `+tc.want, tc.args...)
		})
	}

	value := NewTypedColumn[int64]("v")
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

func TestTypedColumnArithmeticExplicitValues(t *testing.T) {
	calls := 0
	valuer := templateValuer{calls: &calls}
	value := int64(3)
	nullable := sql.NullInt64{Int64: 4, Valid: true}
	b := Update().Table("t").Set(
		NewTypedColumn[int64]("a").AddExpr(Value(valuer)),
		NewTypedColumn[int64]("b").SubExpr(Value(&value)),
		NewTypedColumn[int64]("c").MulExpr(Value(nullable)),
		NewTypedColumn[int64]("d").DivExpr(Value(nil)),
	).SetDialect(dialect.Postgres)

	want := []any{valuer, &value, nullable, nil}
	checkSQL(t, b, `UPDATE "t" SET "a"="a" + ($1), "b"="b" - ($2), "c"="c" * ($3), "d"="d" / ($4)`, want...)
	_, args, err := mustCompileTemplate(t, b).Bind()
	if err != nil || calls != 0 || !reflect.DeepEqual(args, want) {
		t.Fatal("explicit values must bind unchanged without driver conversion", args, err, calls)
	}

	checkSQL(t, Update().Table("t").Set(
		NewTypedColumn[int64]("a").AddExpr(Value(sql.Named("delta", 2))),
		NewTypedColumn[int64]("b").SubExpr(Value(sql.Named("delta", 2))),
	).SetDialect(dialect.SQLite),
		`UPDATE "t" SET "a"="a" + (@delta), "b"="b" - (@delta)`, sql.Named("delta", 2))
}

func TestTypedColumnArithmeticDataTypes(t *testing.T) {
	calls := 0
	valuer := templateValuer{calls: &calls}
	var pointer *int64
	nullable := sql.NullInt64{Int64: 3, Valid: true}
	b := Update().Table("t").Set(
		NewTypedColumn[*int64]("a").Add(nil),
		NewTypedColumn[templateValuer]("b").Sub(valuer),
		NewTypedColumn[sql.NullInt64]("c").Mul(nullable),
		NewTypedColumn[string]("d").Div("2"),
		NewTypedColumn[string]("e").Inc(),
		NewTypedColumn[templateValuer]("f").Dec(),
	).SetDialect(dialect.Postgres)
	want := []any{pointer, valuer, nullable, "2"}
	checkSQL(t, b,
		`UPDATE "t" SET "a"="a" + $1, "b"="b" - $2, "c"="c" * $3, "d"="d" / $4, "e"="e" + 1, "f"="f" - 1`, want...)
	_, args, err := mustCompileTemplate(t, b).Bind()
	if err != nil || calls != 0 || !reflect.DeepEqual(args, want) {
		t.Fatal("typed arithmetic must bind data without numeric or driver conversion", args, err, calls)
	}
}

func TestTypedColumnArithmeticTemplatesAndUpserts(t *testing.T) {
	q := mustCompileTemplate(t, Update().Table("t").
		Set(
			NewTypedColumn[int64]("a").AddExpr(Param(0)),
			NewTypedColumn[int64]("b").SubExpr(Param(1)),
			NewTypedColumn[int64]("c").MulExpr(Param(2)),
			NewTypedColumn[int64]("d").DivExpr(Param(3)),
			NewTypedColumn[int64]("e").Add(2),
			NewTypedColumn[int64]("f").Inc(),
			NewTypedColumn[int64]("g").Dec(),
		).
		Where(NewTypedColumn[int64]("id").EqExpr(Param(4))).
		SetDialect(dialect.Postgres))

	for _, values := range [][]any{{10, 2, 3, 2, 7}, {-3, 0, 4, 5, 8}} {
		query, args, err := q.Bind(values...)
		wantSQL := `UPDATE "t" SET "a"="a" + $1, "b"="b" - $2, "c"="c" * $3, "d"="d" / $4, "e"="e" + $5, "f"="f" + 1, "g"="g" - 1 WHERE ("id" = $6)`
		wantArgs := append(append([]any(nil), values[:4]...), int64(2), values[4])
		if err != nil || query != wantSQL || !reflect.DeepEqual(args, wantArgs) {
			t.Fatal(query, args, err)
		}
	}

	v := NewTypedColumn[int64]("v")
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
