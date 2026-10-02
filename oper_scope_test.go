// Copyright 2026 xgfone
// SPDX-License-Identifier: Apache-2.0

package sqlx

import (
	"fmt"
	"testing"

	"github.com/xgfone/go-sqlx/dialect"
)

func TestOperAppendWhere(t *testing.T) {
	o := NewOper[struct{}]("t").WithDB(&DB{Dialect: dialect.Postgres})
	if o.AppendWhere().AppendWhere(nil, And(), nil) != &o || o.conditions != nil {
		t.Fatal("empty append changed the operation")
	}

	inputs := []Condition{nil, And(Eq("a", 1), nil, And()), Or(Eq("b", 2), Eq("c", 3)), nil}
	calls := 0
	custom := ConditionWriterFunc(func(w *SQLWriter) (bool, error) {
		calls++
		w.Raw(`"d"=`)
		w.Arg(4)
		return true, nil
	})
	if o.AppendWhere(inputs...).AppendWhere(custom) != &o || calls != 0 {
		t.Fatal("append lost receiver identity or evaluated a condition")
	}
	clear(inputs)

	query := o.Select("id")
	o.AppendWhere(nil, And(), nil).AppendWhere(Eq("e", 5))
	query.Where(Eq("query", 6))
	checkSQL(t, query,
		`SELECT "id" FROM "t" WHERE (("a" = $1) AND (("b" = $2) OR ("c" = $3)) AND "d"=$4 AND ("query" = $5))`,
		1, 2, 3, 4, 6)
	checkSQL(t, o.Select("id"),
		`SELECT "id" FROM "t" WHERE (("a" = $1) AND (("b" = $2) OR ("c" = $3)) AND "d"=$4 AND ("e" = $5))`,
		1, 2, 3, 4, 5)
	if calls != 2 {
		t.Fatal("condition was not evaluated once per query", calls)
	}
}

func TestOperCloneAndWhereBranches(t *testing.T) {
	type model struct {
		ID int `sql:"id"`
	}
	base := NewOper[model]("t").WithDB(&DB{Dialect: dialect.Postgres}).
		WithStructSorter(Column("id").Desc()).
		WithSoftCondition(Eq("deleted", false)).
		WithDeletedCondition(Eq("deleted", true)).
		WithBindConfig(BindConfig{ScanOptions: ScanOptions{Nulls: NullError}})
	// The source has spare capacity, so independently appending after Clone
	// must not overwrite another branch's newly appended condition.
	base.AppendWhere(Eq("tenant", 7))
	left, right := base.Clone(), base.Clone()
	scoped := base.Where(Eq("scope", 8))
	active, deleted := base.Active(), base.Deleted()
	query := base.SelectStruct()

	// Clone also owns its retained entries, rather than just limiting capacity.
	left.conditions[0] = Eq("tenant", 99)
	base.AppendWhere(Eq("base", 1))
	left.AppendWhere(Eq("left", 2))
	right.AppendWhere(Eq("right", 3))
	if left.Table != base.Table || left.bindConfig != base.bindConfig ||
		left.binding().ScanOptions.Nulls != NullError || left.SoftDeleteUpdater == nil {
		t.Fatal("clone lost operation configuration")
	}

	checkSQL(t, base.SelectStruct(), `SELECT "id" FROM "t" WHERE (("tenant" = $1) AND ("base" = $2)) ORDER BY "id" DESC`, 7, 1)
	checkSQL(t, left.SelectStruct(), `SELECT "id" FROM "t" WHERE (("tenant" = $1) AND ("left" = $2)) ORDER BY "id" DESC`, 99, 2)
	checkSQL(t, right.SelectStruct(), `SELECT "id" FROM "t" WHERE (("tenant" = $1) AND ("right" = $2)) ORDER BY "id" DESC`, 7, 3)
	checkSQL(t, scoped.SelectStruct(), `SELECT "id" FROM "t" WHERE (("tenant" = $1) AND ("scope" = $2)) ORDER BY "id" DESC`, 7, 8)
	checkSQL(t, active.SelectStruct(), `SELECT "id" FROM "t" WHERE (("tenant" = $1) AND ("deleted" = $2)) ORDER BY "id" DESC`, 7, false)
	checkSQL(t, deleted.SelectStruct(), `SELECT "id" FROM "t" WHERE (("tenant" = $1) AND ("deleted" = $2)) ORDER BY "id" DESC`, 7, true)
	checkSQL(t, query, `SELECT "id" FROM "t" WHERE ("tenant" = $1) ORDER BY "id" DESC`, 7)

	cleared := right.ClearWhere()
	cleared.AppendWhere(Eq("cleared", 4))
	checkSQL(t, cleared.Select("id"), `SELECT "id" FROM "t" WHERE ("cleared" = $1)`, 4)
	checkSQL(t, right.Select("id"), `SELECT "id" FROM "t" WHERE (("tenant" = $1) AND ("right" = $2))`, 7, 3)
}

func TestOperCloneEmptyConditions(t *testing.T) {
	for _, nonNil := range []bool{false, true} {
		base := NewOper[struct{}]("t").WithDB(&DB{Dialect: dialect.Postgres})
		if nonNil {
			base.conditions = make([]Condition, 0, 4)
		}
		clone := base.Clone()
		if (clone.conditions == nil) != (base.conditions == nil) || len(clone.conditions) != 0 {
			t.Fatal("clone changed empty condition storage")
		}
		base.AppendWhere(Eq("base", 1))
		clone.AppendWhere(Eq("clone", 2))
		checkSQL(t, base.Select("id"), `SELECT "id" FROM "t" WHERE ("base" = $1)`, 1)
		checkSQL(t, clone.Select("id"), `SELECT "id" FROM "t" WHERE ("clone" = $1)`, 2)
	}
}

func TestOperScopeAppendAllocations(t *testing.T) {
	for _, prefix := range []int{0, 1, 3} {
		initial := make([]Condition, prefix)
		for i := range initial {
			initial[i] = Eq("tenant", i)
		}

		base := NewOper[struct{}]("t").Where(initial...)
		whereCondition := Eq("scope", 7)
		conditions := []Condition{Eq("a", 1), Eq("b", 2), Eq("c", 3), Eq("d", 4)}
		for _, entry := range []string{"where", "clone", "active", "deleted"} {
			var scope func() Oper[struct{}]
			retained := prefix
			switch entry {
			case "where":
				scope = func() Oper[struct{}] { return base.Where(whereCondition) }
				retained++

			case "clone":
				scope = base.Clone

			case "active":
				scope = base.Active
				retained++

			case "deleted":
				scope = base.Deleted
				retained++
			}

			for _, mode := range []string{"bulk", "incremental"} {
				t.Run(fmt.Sprintf("prefix%d/%s/%s", prefix, entry, mode), func(t *testing.T) {
					// Scope construction and four appended terms should share one
					// allocation, including when an empty Clone starts unallocated.
					allocs := testing.AllocsPerRun(100, func() {
						o := scope()
						if mode == "bulk" {
							o.AppendWhere(conditions...)
						} else {
							for _, condition := range conditions {
								o.AppendWhere(condition)
							}
						}
						operScopeSink = o
					})
					if allocs != 1 ||
						len(operScopeSink.conditions) != retained+len(conditions) ||
						len(base.conditions) != prefix {
						t.Fatalf("got %g allocations and %d terms, want 1 and %d",
							allocs, len(operScopeSink.conditions), retained+len(conditions))
					}
				})
			}
		}
	}
}
