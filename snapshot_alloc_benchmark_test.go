// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"testing"

	"github.com/xgfone/go-sqlx/dialect"
)

var snapshotOper Oper[struct{}]
var snapshotWindow WindowSpec
var snapshotConflict ConflictTarget

func BenchmarkSnapshotConstruction(b *testing.B) {
	b.Run("LikeEscape", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			expressionWorkloadNode = Like("name", "a%", "!")
		}
	})

	e := Ident("score")
	terms := SortColumns{{Column: "id", Order: Asc}, {Expr: &e, Order: Desc}}
	b.Run("Sort", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			performanceBuilder = Select("id").Sort(terms)
		}
	})

	conditions := []Condition{Eq("a", 1), Eq("b", 2), Eq("c", 3)}
	extra := Eq("d", 4)
	b.Run("OperWhere", func(b *testing.B) {
		o := NewOper[struct{}]("t").Where(conditions...)
		b.ReportAllocs()
		for b.Loop() {
			snapshotOper = o.Where(extra)
		}
	})

	b.Run("ConflictWhere", func(b *testing.B) {
		target := ConflictColumns("id").Where(conditions...)
		b.ReportAllocs()
		for b.Loop() {
			snapshotConflict = target.Where(extra)
		}
	})

	b.Run("WindowPartition", func(b *testing.B) {
		w := Window().PartitionBy("a", "b", "c")
		b.ReportAllocs()
		for b.Loop() {
			snapshotWindow = w.PartitionBy("d")
		}
	})

	b.Run("WindowSort", func(b *testing.B) {
		w := Window().Sort(terms)
		b.ReportAllocs()
		for b.Loop() {
			snapshotWindow = w.Sort(terms)
		}
	})

	b.Run("InsertSelectCTE", func(b *testing.B) {
		q := Insert().Into("t").With("outer", Select("id").From("u")).
			FromSelect(Select("id").From("outer").Where(Eq("id", 7))).
			SetDialect(dialect.MySQL)

		b.ReportAllocs()
		for b.Loop() {
			s, args, err := q.Build()
			if err != nil {
				b.Fatal(err)
			}
			expressionHelperSQL, expressionWorkloadArgs = s, args
		}
	})
}
