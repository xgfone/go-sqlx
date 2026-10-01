// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"slices"
	"testing"

	"github.com/xgfone/go-sqlx/dialect"
)

func TestLikeEscapeSnapshot(t *testing.T) {
	for _, tc := range []struct {
		name string
		op   string
		make func([]string) Condition
	}{
		{
			"like", "LIKE", func(escape []string) Condition {
				return Like("name", "a%", escape...)
			},
		},
		{
			"not like", "NOT LIKE", func(escape []string) Condition {
				return NotLike("name", "a%", escape...)
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			escape := []string{"!"}
			condition := tc.make(escape)
			escape[0] = "changed"
			checkSQL(t, Select("id").Where(condition),
				"SELECT `id` WHERE (`name` "+tc.op+" ? ESCAPE ?)", "a%", "!")

			escape[0] = "invalid"
			condition = tc.make(escape)
			escape[0] = "!"
			checkBuildError(t, Select("id").Where(condition))
		})
	}
}

func TestScopeBranchesKeepConditions(t *testing.T) {
	conditions := []Condition{Eq("a", 1), Eq("b", 2), Eq("c", 3)}
	o := NewOper[struct{}]("t").Where(conditions...)
	left, right := o.Where(Eq("d", 4)), o.Where(Eq("e", 5))
	target := ConflictColumns("id").Where(conditions...)
	action := target.DoUpdate(Set("v", 6)).Where(conditions...)
	leftAction, rightAction := action.Where(Eq("d", 4)), action.Where(Eq("e", 5))
	leftTarget, rightTarget := target.Where(Eq("d", 4)), target.Where(Eq("e", 5))
	conditions[0] = Eq("changed", 99)

	checkSQL(t, o.Where().Select("id"), "SELECT `id` FROM `t` WHERE ((`a` = ?) AND (`b` = ?) AND (`c` = ?))", 1, 2, 3)
	checkSQL(t, left.Select("id"), "SELECT `id` FROM `t` WHERE ((`a` = ?) AND (`b` = ?) AND (`c` = ?) AND (`d` = ?))", 1, 2, 3, 4)
	checkSQL(t, right.Select("id"), "SELECT `id` FROM `t` WHERE ((`a` = ?) AND (`b` = ?) AND (`c` = ?) AND (`e` = ?))", 1, 2, 3, 5)

	for _, tc := range []struct {
		target ConflictTarget
		action ConflictClause
		column string
		value  int
	}{
		{leftTarget.Where(), leftAction.Where(), "d", 4},
		{rightTarget, rightAction, "e", 5},
	} {
		checkSQL(t, Insert().Into("t").Values(0).OnConflict(tc.target.DoNothing()).SetDialect(dialect.SQLite),
			`INSERT INTO "t" VALUES (?) ON CONFLICT ("id") WHERE (("a" = ?) AND ("b" = ?) AND ("c" = ?) AND ("`+tc.column+`" = ?)) DO NOTHING`,
			0, 1, 2, 3, tc.value)
		checkSQL(t, Insert().Into("t").Values(0).OnConflict(tc.action).SetDialect(dialect.SQLite),
			`INSERT INTO "t" VALUES (?) ON CONFLICT ("id") WHERE (("a" = ?) AND ("b" = ?) AND ("c" = ?)) DO UPDATE SET "v"=? WHERE (("a" = ?) AND ("b" = ?) AND ("c" = ?) AND ("`+tc.column+`" = ?))`,
			0, 1, 2, 3, 6, 1, 2, 3, tc.value)
	}
}

func TestWindowBranchesKeepSnapshots(t *testing.T) {
	e := Ident("score")
	terms := SortColumns{{Expr: &e, Order: Asc}}
	w := Window().PartitionBy("a", "b", "c").OrderBy("x", Asc).OrderBy("y", Asc).Sort(terms)
	partitions := []Expression{Ident("d")}
	left := w.PartitionByExpr(partitions...).Sort(SortColumn{Column: "first", Order: Desc})
	right := w.PartitionBy("e").Sort(SortColumn{Column: "last", Order: Desc})
	e, partitions[0], terms[0].Order = Ident("changed"), Ident("changed"), Desc

	checkSQL(t, Select().SelectExpr(Sum("n").Over(w.PartitionBy().PartitionByExpr().Sort())).SetDialect(dialect.Postgres),
		`SELECT SUM("n") OVER (PARTITION BY "a", "b", "c" ORDER BY "x" ASC, "y" ASC, "score" ASC)`)
	checkSQL(t, Select().SelectExpr(Sum("n").Over(left)).SetDialect(dialect.Postgres),
		`SELECT SUM("n") OVER (PARTITION BY "a", "b", "c", "d" ORDER BY "x" ASC, "y" ASC, "score" ASC, "first" DESC)`)
	checkSQL(t, Select().SelectExpr(Sum("n").Over(right)).SetDialect(dialect.Postgres),
		`SELECT SUM("n") OVER (PARTITION BY "a", "b", "c", "e" ORDER BY "x" ASC, "y" ASC, "score" ASC, "last" DESC)`)
}

func TestInsertPlanColumnAppendsStayIndependent(t *testing.T) {
	type model struct{ A, B, C int }
	p, err := CompileInsert[model]()
	if err != nil {
		t.Fatal(err)
	}
	// Exercise plans with spare capacity, as can happen with explicit columns.
	p.columns = slices.Grow(p.columns, 8)
	for _, extra := range []string{"first", "second"} {
		t.Run(extra, func(t *testing.T) {
			t.Parallel()
			for range 20 {
				b := Insert().Into("t")
				if err := p.AppendTo(b, []model{{1, 2, 3}}); err != nil {
					t.Fatal(err)
				}
				b.Columns(extra).ClearValues().Values(1, 2, 3, 4)
				checkSQL(t, b, "INSERT INTO `t` (`A`, `B`, `C`, `"+extra+"`) VALUES (?, ?, ?, ?)", 1, 2, 3, 4)
			}
		})
	}
}

func TestInsertSelectRenderingKeepsSourceSnapshot(t *testing.T) {
	for _, tc := range []struct {
		name string
		d    Dialect
		sql  string
	}{
		{"mysql", dialect.MySQL,
			"INSERT INTO `t` (`id`) WITH `outer` AS (SELECT ? AS `id`), `inner` AS (SELECT ? AS `id`) SELECT `id` FROM `inner`"},
		{"sqlite", dialect.SQLite,
			`WITH "outer" AS (SELECT ? AS "id") INSERT INTO "t" ("id") SELECT * FROM (WITH "inner" AS (SELECT ? AS "id") SELECT "id" FROM "inner") AS "_sqlx_insert" WHERE (TRUE) ON CONFLICT DO NOTHING`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := Select("id").From("inner").With("inner", Select().SelectExprAlias(Value(2), "id"))
			b := Insert().Into("t").Columns("id").FromSelect(source).
				With("outer", Select().SelectExprAlias(Value(1), "id")).SetDialect(tc.d)
			if tc.d == dialect.SQLite {
				b.OnConflict(ConflictColumns().DoNothing())
			}
			source.Where(Eq("changed", 99))
			for range 4 {
				t.Run("build", func(t *testing.T) {
					t.Parallel()
					for range 20 {
						checkSQL(t, b, tc.sql, 1, 2)
						checkSQL(t, b.source, "WITH `inner` AS (SELECT ? AS `id`) SELECT `id` FROM `inner`", 2)
					}
				})
			}
		})
	}
}
